package main

import (
	"strings"
)

// nativeBranchLanded answers the case `rev-list --not --remotes=origin` cannot:
// a branch pushed, squash-merged, and then pruned upstream. See docs/native-shadow.md.
func nativeBranchLanded(repository, branch string) bool {
	if !nativeBranchWasPushedAndPruned(repository, branch) {
		return false
	}
	base := nativeDefaultRemoteRef(repository)
	if base == "" {
		return false
	}
	reference := "refs/heads/" + branch
	mergeBase, err := nativeGit(repository, "merge-base", base, reference)
	if err != nil || mergeBase == "" {
		return false
	}
	listing, err := nativeGit(repository, "diff", "--name-only", mergeBase, reference)
	if err != nil {
		return false
	}
	paths := []string{}
	for _, path := range strings.Split(listing, "\n") {
		if path = strings.TrimSpace(path); path != "" {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return true
	}
	// Squashing rewrites every commit, so patch identity cannot answer this.
	// Content can: the branch is spent when it changes nothing main lacks.
	arguments := append([]string{"diff", "--quiet", base, reference, "--"}, paths...)
	_, err = nativeGit(repository, arguments...)
	return err == nil
}

// A branch that was never pushed may hold the only copy of its commits, so it
// stays out of this path however finished it looks.
func nativeBranchWasPushedAndPruned(repository, branch string) bool {
	merge, err := nativeGit(repository, "config", "--get", "branch."+branch+".merge")
	if err != nil || merge == "" {
		return false
	}
	remote, err := nativeGit(repository, "config", "--get", "branch."+branch+".remote")
	if err != nil || remote == "" {
		return false
	}
	tracked := "refs/remotes/" + remote + "/" + strings.TrimPrefix(merge, "refs/heads/")
	_, err = nativeGit(repository, "show-ref", "--verify", "--quiet", tracked)
	return err != nil
}

// nativeDefaultRemoteRef prefers the remote's own head over local main, which a
// stale checkout can leave behind the branch being judged.
func nativeDefaultRemoteRef(repository string) string {
	if head, err := nativeGit(
		repository, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD",
	); err == nil && head != "" {
		return head
	}
	if _, err := nativeGit(
		repository, "show-ref", "--verify", "--quiet", "refs/remotes/origin/main",
	); err == nil {
		return "origin/main"
	}
	return ""
}

// Wider than nativeBranchLanded, which deletes a ref rather than silencing a
// warning. Needs git 2.43 for `-X`. docs/native-shadow.md
func nativeBranchSubsumed(repository, branch string) bool {
	base := nativeDefaultRemoteRef(repository)
	if base == "" {
		return false
	}
	landed, err := nativeGit(repository, "rev-parse", base+"^{tree}")
	if err != nil || landed == "" {
		return false
	}
	merged, err := nativeGit(repository, "merge-tree", "--write-tree",
		"-X", "ours", base, "refs/heads/"+branch)
	if err != nil {
		return false
	}
	merged, _, _ = strings.Cut(merged, "\n")
	return merged == landed
}
