package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func checkFixture(t *testing.T) (bundleItem, string) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("bundles are macOS only")
	}
	root := t.TempDir()
	terminal := filepath.Join(root, "kitty-real")
	if err := os.WriteFile(terminal, []byte{0xcf, 0xfa, 0xed, 0xfe}, 0o755); err != nil {
		t.Fatalf("write the terminal fixture: %v", err)
	}
	spec := testSpec()
	item := bundleItem{
		Role:       spec.Role,
		Person:     spec.Person,
		Identifier: spec.identifier(),
		Path:       filepath.Join(root, spec.name()+".app"),
		Executable: spec.executable(),
		Terminal:   terminal,
		Launcher:   bundleLauncher(spec),
		Plist:      bundleInfoPlist(spec),
	}
	if err := writeBundle(item, "", defaultBundleTag); err != nil {
		t.Fatalf("write the bundle: %v", err)
	}
	return item, root
}

// A check that flags a fresh bundle would make convergence rewrite a launcher
// that is running. See docs/aterm-bundles.md.
func TestBundleDriftIsEmptyForABundleJustWritten(t *testing.T) {
	item, _ := checkFixture(t)
	drift, err := bundleDrift(item, "", defaultBundleTag)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(drift) != 0 {
		t.Fatalf("a bundle written from this plan drifted: %v", drift)
	}
}

func TestBundleDriftNamesEachDifferingPart(t *testing.T) {
	item, root := checkFixture(t)
	contents := filepath.Join(item.Path, "Contents")
	cases := []struct {
		name   string
		mutate func(bundleItem) bundleItem
		want   string
	}{
		{"launcher", func(i bundleItem) bundleItem { i.Launcher += "# changed\n"; return i }, "launcher"},
		{"plist", func(i bundleItem) bundleItem { i.Plist += "<!-- changed -->"; return i }, "Info.plist"},
		{"terminal", func(i bundleItem) bundleItem { i.Terminal = filepath.Join(root, "other"); return i }, "terminal link"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			drift, err := bundleDrift(tc.mutate(item), "", defaultBundleTag)
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			if strings.Join(drift, ",") != tc.want {
				t.Fatalf("drift = %v, want only %q", drift, tc.want)
			}
		})
	}
	t.Run("tag", func(t *testing.T) {
		drift, err := bundleDrift(item, "", "another")
		if err != nil {
			t.Fatalf("check: %v", err)
		}
		if strings.Join(drift, ",") != "Finder tag" {
			t.Fatalf("drift = %v", drift)
		}
	})
	t.Run("icon", func(t *testing.T) {
		shared := filepath.Join(root, "shared.icns")
		if err := os.WriteFile(shared, []byte("icns"), 0o644); err != nil {
			t.Fatal(err)
		}
		drift, err := bundleDrift(item, shared, defaultBundleTag)
		if err != nil {
			t.Fatalf("check: %v", err)
		}
		if strings.Join(drift, ",") != "icon" {
			t.Fatalf("drift = %v", drift)
		}
	})
	t.Run("hand edit on disk", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(contents, "MacOS", item.Executable),
			[]byte("#!/bin/sh\n"+bundleMarker+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		drift, err := bundleDrift(item, "", defaultBundleTag)
		if err != nil {
			t.Fatalf("check: %v", err)
		}
		if strings.Join(drift, ",") != "launcher" {
			t.Fatalf("drift = %v", drift)
		}
	})
}

func TestBundleDriftReportsAMissingBundleAndRefusesAForeignOne(t *testing.T) {
	item, _ := checkFixture(t)
	missing := item
	missing.Path = filepath.Join(filepath.Dir(item.Path), "Absent.app")
	drift, err := bundleDrift(missing, "", "")
	if err != nil || strings.Join(drift, ",") != "missing" {
		t.Fatalf("missing bundle: drift %v, err %v", drift, err)
	}

	foreign := filepath.Join(filepath.Dir(item.Path), "Foreign.app")
	if err := os.MkdirAll(filepath.Join(foreign, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	other := item
	other.Path = foreign
	if _, err := bundleDrift(other, "", ""); err == nil {
		t.Fatal("an app this command did not write should be refused, as a write refuses it")
	}
}

// The exit code is what lets a convergence tool tell drift from a failure.
func TestCheckBundlesExitsWithDriftOnlyWhenABundleDiffers(t *testing.T) {
	item, _ := checkFixture(t)
	plan := bundlePlan{Items: []bundleItem{item}, Tag: defaultBundleTag}

	var out bytes.Buffer
	if err := checkBundles(&out, plan); err != nil {
		t.Fatalf("a current bundle should pass: %v", err)
	}
	if !strings.HasPrefix(out.String(), "current "+item.Path) {
		t.Fatalf("output = %q", out.String())
	}

	plan.Items[0].Launcher += "# changed\n"
	out.Reset()
	err := checkBundles(&out, plan)
	if err == nil || exitCodeFor(err) != exitDrift {
		t.Fatalf("drift should exit %d, got %v", exitDrift, err)
	}
	if !strings.Contains(out.String(), "drifted "+item.Path+": launcher") {
		t.Fatalf("output = %q", out.String())
	}
	var typed exitError
	if !errors.As(err, &typed) {
		t.Fatalf("the drift error should carry its exit code: %v", err)
	}
}

func TestCheckBundlesReportsAStaleBundleWithoutCountingItAsDrift(t *testing.T) {
	item, _ := checkFixture(t)
	plan := bundlePlan{
		Items: []bundleItem{item},
		Tag:   defaultBundleTag,
		Stale: []string{"/Applications/Old.app"},
	}
	var out bytes.Buffer
	if err := checkBundles(&out, plan); err != nil {
		t.Fatalf("a stale bundle is a report, not drift: %v", err)
	}
	if !strings.Contains(out.String(), "stale, this run does not write it: /Applications/Old.app") {
		t.Fatalf("output = %q", out.String())
	}
}
