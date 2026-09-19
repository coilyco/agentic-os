package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// bundleDrift names each way an installed bundle differs from what writeBundle
// would produce now. It compares the fields writeBundle emits.
func bundleDrift(item bundleItem, icon, tag string) ([]string, error) {
	if _, err := os.Stat(item.Path); os.IsNotExist(err) {
		return []string{"missing"}, nil
	}
	if err := replaceable(item.Path); err != nil {
		return nil, err
	}
	contents := filepath.Join(item.Path, "Contents")
	var drift []string
	if !fileHolds(filepath.Join(contents, "MacOS", item.Executable), []byte(item.Launcher)) {
		drift = append(drift, "launcher")
	}
	if !fileHolds(filepath.Join(contents, "Info.plist"), []byte(item.Plist)) {
		drift = append(drift, "Info.plist")
	}
	if item.Terminal != "" {
		target, err := os.Readlink(filepath.Join(contents, "MacOS", bundleTerminalName))
		if err != nil || target != item.Terminal {
			drift = append(drift, "terminal link")
		}
	}
	art, err := expectedIcon(item.Role, icon)
	if err != nil {
		return nil, err
	}
	if art != nil && !fileHolds(filepath.Join(contents, "Resources", bundleIconName+".icns"), art) {
		drift = append(drift, "icon")
	}
	if want := bundleTagDocument(tag); want != "" && installedBundleTag(item.Path) != want {
		drift = append(drift, "Finder tag")
	}
	return drift, nil
}

// expectedIcon mirrors writeBundle's choice: the shared icon wins, then the
// role's committed art, and nil means the bundle carries none.
func expectedIcon(role, shared string) ([]byte, error) {
	if shared != "" {
		raw, err := os.ReadFile(shared)
		if err != nil {
			return nil, fmt.Errorf("read the icon %q: %w", shared, err)
		}
		return raw, nil
	}
	return roleIcon(role), nil
}

func fileHolds(path string, want []byte) bool {
	got, err := os.ReadFile(path)
	return err == nil && bytes.Equal(got, want)
}

// checkBundles is `bundles --check`, exiting exitDrift when any bundle differs.
// A stale bundle is listed and never counts as drift. See docs/aterm-bundles.md.
func checkBundles(writer io.Writer, plan bundlePlan) error {
	drifted := 0
	for _, item := range plan.Items {
		drift, err := bundleDrift(item, plan.Icon, plan.Tag)
		if err != nil {
			return err
		}
		if len(drift) == 0 {
			fmt.Fprintf(writer, "current %s\n", item.Path)
			continue
		}
		drifted++
		fmt.Fprintf(writer, "drifted %s: %s\n", item.Path, strings.Join(drift, ", "))
	}
	for _, path := range plan.Stale {
		fmt.Fprintf(writer, "stale, this run does not write it: %s\n", path)
	}
	if err := warnStaleLauncher(writer, plan); err != nil {
		return err
	}
	if drifted > 0 {
		return withExit(exitDrift, fmt.Errorf("%d of %d bundles differ from what a write would produce",
			drifted, len(plan.Items)))
	}
	return nil
}
