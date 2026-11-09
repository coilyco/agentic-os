package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Work a seat holds that its state does not show. See references/seat-state.md.

// footerSegment is a numbered footer entry, `1 shell` or `1 monitor`. `← 1 agent` is not
// one: it starts with an arrow and read 1 on a seat running nothing.
var footerSegment = regexp.MustCompile(`^(\d+) [a-z][a-z ]*$`)

// backgroundCount is the tasks claude's `⏵⏵` footer counts, and false when the footer is
// not that or is cut with `…`, which a client reads as unknown.
func backgroundCount(seat string, lines []string) (int, bool) {
	if seat != "claude" {
		return 0, false
	}
	footer := ""
	for index := len(lines) - 1; index >= 0; index-- {
		if strings.TrimSpace(lines[index]) != "" {
			footer = strings.TrimSpace(lines[index])
			break
		}
	}
	if !strings.HasPrefix(footer, "⏵⏵") || strings.HasSuffix(footer, "…") {
		return 0, false
	}
	total := 0
	for _, part := range strings.Split(footer, " · ")[1:] {
		if match := footerSegment.FindStringSubmatch(strings.TrimSpace(part)); match != nil {
			count, _ := strconv.Atoi(match[1])
			total += count
		}
	}
	return total, true
}

const (
	// unpushedDepth is how far below a seat's directory its checkouts are looked for:
	// a native session's directory holds `<owner>/<repo>`.
	unpushedDepth = 2
	// unpushedRepos and unpushedBudget cap the work of one close.
	unpushedRepos  = 40
	unpushedBudget = 8 * time.Second
)

// checkouts are the git checkouts at or up to unpushedDepth below root.
func checkouts(root string) []string {
	var found []string
	pattern := filepath.Join(root, ".git")
	for depth := 0; depth <= unpushedDepth && len(found) < unpushedRepos; depth++ {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			found = append(found, filepath.Dir(match))
		}
		pattern = filepath.Join(root, strings.Repeat("*/", depth+1), ".git")
	}
	if len(found) > unpushedRepos {
		found = found[:unpushedRepos]
	}
	return found
}

// unpushedWork names the first checkout under root holding work no remote has, or "".
// A checkout git cannot read counts as holding nothing, since a refusal needs evidence.
func unpushedWork(root string) string {
	if root == "" || filepath.Clean(root) == string(filepath.Separator) {
		return "" // a seat started at / names no project, and its depth-2 reach would find unrelated clones
	}
	ctx, cancel := context.WithTimeout(context.Background(), unpushedBudget)
	defer cancel()
	for _, repo := range checkouts(root) {
		commits := gitCount(ctx, repo, "rev-list", "--count", "HEAD", "--branches", "--not", "--remotes")
		changed := gitLines(ctx, repo, "status", "--porcelain")
		if commits == 0 && changed == 0 {
			continue
		}
		var parts []string
		if commits > 0 {
			parts = append(parts, fmt.Sprintf("%d commit(s) on no remote", commits))
		}
		if changed > 0 {
			parts = append(parts, fmt.Sprintf("%d changed file(s)", changed))
		}
		return fmt.Sprintf("%s (%s)", repo, strings.Join(parts, ", "))
	}
	return ""
}

func gitOutput(ctx context.Context, repo string, args ...string) string {
	output, err := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func gitCount(ctx context.Context, repo string, args ...string) int {
	count, _ := strconv.Atoi(gitOutput(ctx, repo, args...))
	return count
}

func gitLines(ctx context.Context, repo string, args ...string) int {
	output := gitOutput(ctx, repo, args...)
	if output == "" {
		return 0
	}
	return len(strings.Split(output, "\n"))
}

// knownOrigin keeps a launcher's origin only when it is one of the two values.
func knownOrigin(origin string) string {
	if origin == originWindow || origin == originHeadless {
		return origin
	}
	return ""
}
