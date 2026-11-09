package main

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
)

const skipProfileFixture = `roles:
  platform:
    agent: claude
permissions:
  claude: skip
`

const noPermissionsProfileFixture = `roles:
  platform:
    agent: claude
`

func TestLoadHarnessLaunchProfilesPermissions(t *testing.T) {
	t.Parallel()
	document, err := loadHarnessLaunchProfiles([]byte(skipProfileFixture))
	if err != nil {
		t.Fatal(err)
	}
	if document.Permissions["claude"] != permissionModeSkip {
		t.Fatalf("permissions were not decoded: %+v", document.Permissions)
	}
	for _, body := range []string{
		"roles:\n  platform:\n    agent: claude\npermissions:\n  codex: skip\n",
		"roles:\n  platform:\n    agent: claude\npermissions:\n  claude: bypass\n",
		"roles:\n  platform:\n    agent: claude\npermissions:\n  claude: \"\"\n",
	} {
		if _, err := loadHarnessLaunchProfiles([]byte(body)); err == nil {
			t.Errorf("loadHarnessLaunchProfiles accepted %q", body)
		}
	}
}

func TestApplyConfiguredPermissionModeInsertsAfterTheHarness(t *testing.T) {
	writeProfilesFixture(t, skipProfileFixture)
	for name, tc := range map[string]struct{ in, want []string }{
		"native bare harness": {
			[]string{"claude", "-p", "hi"},
			[]string{"claude", claudeSkipPermissionsFlag, "-p", "hi"},
		},
		"native agent-compose": {
			[]string{"agent-compose", "launch", "platform", "claude", "-p", "hi"},
			[]string{"agent-compose", "launch", "platform", "claude", claudeSkipPermissionsFlag, "-p", "hi"},
		},
		"native spec argv": {
			[]string{"claude", "--name", "seat", "--settings", "s.json"},
			[]string{"claude", claudeSkipPermissionsFlag, "--name", "seat", "--settings", "s.json"},
		},
		"container absolute path": {
			[]string{"/usr/bin/claude"},
			[]string{"/usr/bin/claude", claudeSkipPermissionsFlag},
		},
		"trailing prompt stays last": {
			[]string{"claude", "--", "--permission-mode"},
			[]string{"claude", claudeSkipPermissionsFlag, "--", "--permission-mode"},
		},
	} {
		got, err := applyConfiguredPermissionMode(tc.in, "claude")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: argv = %v, want %v", name, got, tc.want)
		}
	}
}

func TestApplyConfiguredPermissionModeLeavesWhatItShouldAlone(t *testing.T) {
	writeProfilesFixture(t, skipProfileFixture)
	for name, tc := range map[string]struct {
		command []string
		harness string
	}{
		"typed skip flag wins":       {[]string{"claude", claudeSkipPermissionsFlag}, "claude"},
		"typed permission mode wins": {[]string{"claude", "--permission-mode", "plan"}, "claude"},
		"typed permission mode eq":   {[]string{"claude", "--permission-mode=plan"}, "claude"},
		"another harness":            {[]string{"codex", "--version"}, "codex"},
		"not launching the harness":  {[]string{"sh", "-c", "true"}, "claude"},
	} {
		got, err := applyConfiguredPermissionMode(tc.command, tc.harness)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !slices.Equal(got, tc.command) {
			t.Errorf("%s: argv = %v, want it untouched", name, got)
		}
	}
}

// The unpromoted state: a build with this code and no permissions key.
func TestApplyConfiguredPermissionModeIsInertWithoutTheKey(t *testing.T) {
	writeProfilesFixture(t, noPermissionsProfileFixture)
	command := []string{"claude", "-p", "hi"}
	got, err := applyConfiguredPermissionMode(command, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, command) {
		t.Fatalf("argv = %v, want it untouched", got)
	}
}

func TestApplyConfiguredPermissionModeRefusesAnInvalidProfile(t *testing.T) {
	writeProfilesFixture(t, "roles:\n  platform:\n    agent: claude\npermissions:\n  claude: bypass\n")
	if _, err := applyConfiguredPermissionMode([]string{"claude"}, "claude"); err == nil ||
		!strings.Contains(err.Error(), "bypass") {
		t.Fatalf("invalid permissions error = %v, want a refusal naming the value", err)
	}
}

func runClaudeStandaloneDryRun(t *testing.T) string {
	t.Helper()
	useStandaloneWorkspaceFixture(t)
	command := newCommand()
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	err := command.Run(context.Background(), []string{
		"aos",
		"--agent", "claude",
		"--role", "platform",
		"--image", "aos:test",
		"--auth=false",
		"--dry-run",
		"--",
		"--version",
	})
	if err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestStandaloneContainerDryRunCarriesTheSkipFlagOnlyWhenPinned(t *testing.T) {
	writeProfilesFixture(t, skipProfileFixture)
	if rendered := runClaudeStandaloneDryRun(t); !strings.Contains(rendered, "-- claude "+claudeSkipPermissionsFlag+" --version") {
		t.Errorf("pinned dry run missing the skip flag:\n%s", rendered)
	}
	writeProfilesFixture(t, noPermissionsProfileFixture)
	if rendered := runClaudeStandaloneDryRun(t); strings.Contains(rendered, claudeSkipPermissionsFlag) {
		t.Errorf("unpinned dry run carries the skip flag:\n%s", rendered)
	}
}
