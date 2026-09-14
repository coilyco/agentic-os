// Materialises a skill root owned by another repository, so a private repo can
// reach a public consumer without a submodule. See the skill-mounts doc.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/urfave/cli/v3"
)

const skillMountManifest = ".agents/skill-mounts.yaml"

// skillMount names a skill root this repo consumes and does not own. It carries
// no clone URL and no credential, so the manifest stays public-safe.
type skillMount struct {
	Root   string `yaml:"root"`
	Repo   string `yaml:"repo"`
	Source string `yaml:"source"`
}

type skillMountFile struct {
	Mounts []skillMount `yaml:"mounts"`
}

var (
	skillRootRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	skillRepoRe = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)
)

func (m skillMount) validate() error {
	if !skillRootRe.MatchString(m.Root) {
		return fmt.Errorf("root %q: lowercase letters, digits and hyphens", m.Root)
	}
	if !skillRepoRe.MatchString(m.Repo) {
		return fmt.Errorf("repo %q: want owner/name", m.Repo)
	}
	source := filepath.Clean(m.Source)
	if m.Source == "" || strings.HasPrefix(source, "..") || filepath.IsAbs(source) {
		return fmt.Errorf("source %q: a relative path inside the source repo", m.Source)
	}
	return nil
}

func readSkillMounts(path string) ([]skillMount, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var file skillMountFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	seen := map[string]bool{}
	for _, mount := range file.Mounts {
		if err := mount.validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if seen[mount.Root] {
			return nil, fmt.Errorf("%s: root %q declared twice", path, mount.Root)
		}
		seen[mount.Root] = true
	}
	return file.Mounts, nil
}

// mountTarget is where a root lands. Kept beside the repo's own skills, because
// the consumer reads one directory and should not learn which are borrowed.
func mountTarget(repoRoot string, mount skillMount) string {
	return filepath.Join(repoRoot, ".agents", "skills", mount.Root)
}

func skillMountPresent(repoRoot string, mount skillMount) bool {
	info, err := os.Stat(mountTarget(repoRoot, mount))
	return err == nil && info.IsDir()
}

// ignoredByGit reports whether a mounted root would be tracked, which would
// commit another repo's content into this one.
func ignoredByGit(repoRoot, target string) bool {
	// Probed inside the target: a `dir/` rule cannot match a directory that
	// does not exist yet, which is every first mount.
	probe := filepath.Join(target, "SKILL.md")
	cmd := exec.Command("git", "-C", repoRoot, "check-ignore", "-q", probe)
	return cmd.Run() == nil
}

func cloneSkillSource(ctx context.Context, forge, repo, into string) error {
	url := strings.TrimSuffix(forge, "/") + "/" + repo + ".git"
	// Whatever credential the operator already holds for the forge. Nothing is
	// read from, or written to, the manifest.
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--quiet", url, into)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clone %s: %w", repo, err)
	}
	return nil
}

func copyTree(from, to string) error {
	if err := os.RemoveAll(to); err != nil {
		return fmt.Errorf("clear %s: %w", to, err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(to), err)
	}
	return filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode().Perm())
	})
}

type skillMountStatus struct {
	Mount   skillMount
	Present bool
	Ignored bool
}

func skillMountStatuses(repoRoot string, mounts []skillMount) []skillMountStatus {
	out := make([]skillMountStatus, 0, len(mounts))
	for _, mount := range mounts {
		out = append(out, skillMountStatus{
			Mount:   mount,
			Present: skillMountPresent(repoRoot, mount),
			Ignored: ignoredByGit(repoRoot, mountTarget(repoRoot, mount)),
		})
	}
	return out
}

func runSkillMount(ctx context.Context, repoRoot, forge string, check bool, out *os.File) error {
	manifest := filepath.Join(repoRoot, skillMountManifest)
	mounts, err := readSkillMounts(manifest)
	if err != nil {
		return err
	}
	if len(mounts) == 0 {
		fmt.Fprintf(out, "%s declares no mounts\n", skillMountManifest)
		return nil
	}
	statuses := skillMountStatuses(repoRoot, mounts)
	if check {
		missing := 0
		for _, status := range statuses {
			state := "mounted"
			if !status.Present {
				state = "absent"
				missing++
			}
			fmt.Fprintf(out, "%-32s %-8s from %s\n", status.Mount.Root, state, status.Mount.Repo)
		}
		if missing > 0 {
			return fmt.Errorf(
				"%d of %d skill mount(s) absent: run `aos skills mount` with access to the "+
					"source repositories", missing, len(statuses))
		}
		return nil
	}
	for _, status := range statuses {
		target := mountTarget(repoRoot, status.Mount)
		// Refusing here rather than after the copy: a tracked target would put
		// another repo's content into this one's history.
		if !status.Ignored {
			return fmt.Errorf(
				"%s is not ignored by git, so mounting it would commit %s into this repo. "+
					"Add it to .gitignore first",
				filepath.Join(".agents", "skills", status.Mount.Root), status.Mount.Repo)
		}
		stage, err := os.MkdirTemp("", "aos-skill-mount-")
		if err != nil {
			return fmt.Errorf("stage %s: %w", status.Mount.Root, err)
		}
		err = func() error {
			defer os.RemoveAll(stage)
			if err := cloneSkillSource(ctx, forge, status.Mount.Repo, stage); err != nil {
				return err
			}
			source := filepath.Join(stage, filepath.Clean(status.Mount.Source))
			info, statErr := os.Stat(source)
			if statErr != nil || !info.IsDir() {
				return fmt.Errorf("%s: %s is not a directory in %s",
					status.Mount.Root, status.Mount.Source, status.Mount.Repo)
			}
			return copyTree(source, target)
		}()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "mounted %-32s from %s/%s\n",
			status.Mount.Root, status.Mount.Repo, status.Mount.Source)
	}
	return nil
}

func skillsCommand() *cli.Command {
	return &cli.Command{
		Name:  "skills",
		Usage: "materialise skill roots this repo consumes and does not own",
		Commands: []*cli.Command{
			{
				Name:  "mount",
				Usage: "clone each declared source and copy its skill root into .agents/skills",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "check", Usage: "report what is mounted without fetching"},
					&cli.StringFlag{Name: "repo-root", Value: ".", Usage: "the consuming checkout"},
					&cli.StringFlag{
						Name:  "forge",
						Value: "https://forgejo.coilysiren.me",
						Usage: "forge base URL the sources are cloned from",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					root, err := filepath.Abs(cmd.String("repo-root"))
					if err != nil {
						return err
					}
					return runSkillMount(ctx, root, cmd.String("forge"), cmd.Bool("check"), os.Stdout)
				},
			},
		},
	}
}
