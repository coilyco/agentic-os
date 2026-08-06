package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useStandaloneWorkspaceFixture(t *testing.T) (nativeRuntime, string) {
	t.Helper()
	root := t.TempDir()
	repository, _ := createNativeTestRepository(t, root, "owner", "one")
	profiles := filepath.Join(repository, harnessLaunchProfilesRelativePath)
	if err := os.MkdirAll(filepath.Dir(profiles), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profiles, []byte(`roles:
  engineer:
    agent: codex
  director:
    agent: claude
  qa:
    agent: codex
  ops:
    agent: claude
  design:
    agent: claude
  community:
    agent: claude
  strats:
    agent: claude
  content:
    agent: codex
`), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, repository, "add", filepath.ToSlash(harnessLaunchProfilesRelativePath))
	testGit(t, repository, "commit", "-m", "add harness launch profiles")
	testGit(t, repository, "push", "origin", "main")
	runtime := nativeTestRuntime(t, root)
	writeNativeTestPlan(t, runtime.PlanFile, "one")
	writeNativeTestList(t, runtime.FleetFile, "owner")
	t.Setenv("AOS_REPOSITORY_PLAN", runtime.PlanFile)
	t.Setenv("PROJECTS_ROOT", runtime.ProjectsRoot)
	t.Setenv("AOS_NATIVE_STATE_DIR", runtime.StateRoot)
	t.Setenv("AOS_NATIVE_SESSIONS_DIR", runtime.SessionsRoot)
	t.Setenv("HOME", runtime.Home)
	t.Setenv("USERPROFILE", runtime.Home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(runtime.Home, ".config"))
	t.Chdir(repository)
	return runtime, repository
}

func useStandaloneWorkspaceWithoutRepositoryPlanFixture(t *testing.T) nativeRuntime {
	t.Helper()
	root := t.TempDir()
	runtime := nativeTestRuntime(t, root)
	scratch := filepath.Join(root, "scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	profiles := filepath.Join(root, "harness-launch-profiles.yaml")
	if err := os.WriteFile(profiles, []byte(`roles:
  engineer:
    agent: codex
`), 0o644); err != nil {
		t.Fatal(err)
	}
	runtime.CWD = scratch
	t.Setenv("AOS_HARNESS_LAUNCH_PROFILES", profiles)
	t.Setenv("AOS_REPOSITORY_PLAN", runtime.PlanFile)
	t.Setenv("PROJECTS_ROOT", runtime.ProjectsRoot)
	t.Setenv("AOS_NATIVE_STATE_DIR", runtime.StateRoot)
	t.Setenv("AOS_NATIVE_SESSIONS_DIR", runtime.SessionsRoot)
	t.Setenv("HOME", runtime.Home)
	t.Setenv("USERPROFILE", runtime.Home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(runtime.Home, ".config"))
	t.Chdir(scratch)
	return runtime
}

func addNativeTestDirectory(t *testing.T, repository, relative string) string {
	t.Helper()
	directory := filepath.Join(repository, filepath.FromSlash(relative))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "README.md"), []byte(relative+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, repository, "add", filepath.ToSlash(filepath.Join(relative, "README.md")))
	testGit(t, repository, "commit", "-m", "add "+relative)
	testGit(t, repository, "push", "origin", "main")
	return directory
}

func TestValidateIntegratedLaunchMatrix(t *testing.T) {
	t.Parallel()
	valid := []integratedLaunchOptions{
		{
			Image: "aos:test", Role: "engineer", Agent: "codex",
			Delivery: "native-skills", Composed: true,
		},
		{
			Image: "aos:test", Role: "design", Agent: "claude",
			Delivery: "native-skills", Guarded: true,
		},
		{
			Image: "aos:test", Role: "director", Agent: "goose",
			Delivery: "compiled", Warded: true,
		},
		{
			Image: "aos:test", Role: "qa", Agent: "opencode",
			Delivery: "compiled", Warded: true, Composed: true,
			Arguments: []string{"owner/repo#123"},
		},
		{
			Image: "aos:test", Role: "story-architect", Agent: "codex",
			Delivery: "compiled", Warded: true, Composed: true,
			AgentID: "architect", Arguments: []string{"shape the premise"},
		},
	}
	for _, opts := range valid {
		if err := validateIntegratedLaunch(opts); err != nil {
			t.Errorf("valid launch %+v failed: %v", opts, err)
		}
	}

	invalid := []struct {
		name string
		opts integratedLaunchOptions
		want string
	}{
		{
			name: "missing role",
			opts: integratedLaunchOptions{
				Image: "aos:test", Agent: "codex", Composed: true,
			},
			want: "needs --role",
		},
		{
			name: "unknown agent",
			opts: integratedLaunchOptions{
				Image: "aos:test", Role: "engineer", Agent: "other", Composed: true,
			},
			want: "unsupported --agent",
		},
		{
			name: "generic role missing work",
			opts: integratedLaunchOptions{
				Image: "aos:test", Role: "design", Agent: "codex", Warded: true,
			},
			want: "needs work text",
		},
		{
			name: "fixed workflow agent id",
			opts: integratedLaunchOptions{
				Image: "aos:test", Role: "engineer", Agent: "codex", Warded: true,
				AgentID: "engineer-one", Arguments: []string{"owner/repo#1"},
			},
			want: "only for generic warded roles",
		},
		{
			name: "missing work",
			opts: integratedLaunchOptions{
				Image: "aos:test", Role: "engineer", Agent: "codex", Warded: true,
			},
			want: "needs an issue reference or freeform work",
		},
		{
			name: "authority translation override",
			opts: integratedLaunchOptions{
				Image: "aos:test", Role: "qa", Agent: "codex", Warded: true,
				Arguments: []string{"owner/repo#1", "--context-bundle", "other"},
			},
			want: "conflicts with AOS-owned Ward translation",
		},
		{
			name: "substrate without composition",
			opts: integratedLaunchOptions{
				Image: "aos:test", Role: "engineer", Agent: "codex",
				Guarded: true, NoSubstrate: true,
			},
			want: "needs --composed",
		},
		{
			name: "warded kubeconfig",
			opts: integratedLaunchOptions{
				Image: "aos:test", Role: "director", Agent: "codex",
				Warded: true, Kubeconfig: "/host/config",
			},
			want: "only for standalone launches",
		},
	}
	for _, test := range invalid {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateIntegratedLaunch(test.opts)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildWardLaunchPlanUsesGenericRunForArbitraryComposedRole(t *testing.T) {
	t.Parallel()
	plan, err := buildWardLaunchPlan(integratedLaunchOptions{
		Image:     "aos:test",
		Role:      "story-architect",
		AgentID:   "architect",
		Agent:     "codex",
		Arguments: []string{"shape the premise"},
	}, "/cache/context")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(append([]string{plan.Command}, plan.Args...), " ")
	for _, want := range []string{
		"ward agent run --role story-architect --agent-id architect shape the premise",
		"--agent codex",
		"--image aos:test",
		"--context-bundle /cache/context",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Ward launch %q does not contain %q", got, want)
		}
	}
}

func TestBuildWardLaunchPlanOwnsSiblingTranslation(t *testing.T) {
	t.Parallel()
	plan, err := buildWardLaunchPlan(integratedLaunchOptions{
		Image:     "aos:test",
		Role:      "engineer",
		Agent:     "codex",
		Arguments: []string{"owner/repo#267", "--print"},
	}, "/cache/context")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(append([]string{plan.Command}, plan.Args...), " ")
	for _, want := range []string{
		"ward agent engineer owner/repo#267 --print",
		"--agent codex",
		"--image aos:test",
		"--context-bundle /cache/context",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Ward launch %q does not contain %q", got, want)
		}
	}
	if len(plan.Environment) != 0 {
		t.Fatalf("Ward launch received AOS-owned harness environment: %v", plan.Environment)
	}
	if strings.Contains(got, "--config") {
		t.Fatalf("Ward launch contains a workflow-local config flag: %q", got)
	}
}

func TestIntegratedWardedDirectorCodexDryRunUsesOpaqueCompositionMetadata(t *testing.T) {
	t.Parallel()
	command := newCommand()
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	err := command.Run(context.Background(), []string{
		"aos",
		"--agent", "codex",
		"--role", "director",
		"--image", "aos:test",
		"--warded",
		"--composed",
		"--guarded",
		"--dry-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, want := range []string{"ward agent director", "--agent codex", "--context-bundle '<AOS_CONTEXT_BUNDLE>'"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("dry run missing %q:\n%s", want, rendered)
		}
	}
	for _, retired := range []string{
		"WARD_CODEX_MODEL=",
		"WARD_CODEX_REASONING_EFFORT=",
		"WARD_CODEX_VERBOSITY=",
	} {
		if strings.Contains(rendered, retired) {
			t.Fatalf("Ward launch retained AOS-owned harness environment %q:\n%s", retired, rendered)
		}
	}
	if strings.Contains(rendered, "--config") {
		t.Fatalf("Ward launch contains a workflow-local config flag:\n%s", rendered)
	}
}

func TestIntegratedWardedDryRunStartsNoProcess(t *testing.T) {
	t.Parallel()
	command := newCommand()
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	err := command.Run(context.Background(), []string{
		"aos",
		"--agent", "codex",
		"--role", "engineer",
		"--image", "aos:test",
		"--warded",
		"--composed",
		"--guarded",
		"--dry-run",
		"--",
		"owner/repo#267",
		"--print",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, want := range []string{
		"docker run",
		"_container-context-bundle",
		"--composed",
		"--guarded",
		"ward agent engineer",
		"--agent codex",
		"--image aos:test",
		"--context-bundle '<AOS_CONTEXT_BUNDLE>'",
		"owner/repo#267",
		"--print",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("dry run missing %q:\n%s", want, rendered)
		}
	}
}

func TestIntegratedStandaloneDryRunAlwaysUsesComposedAndGuardedContexts(t *testing.T) {
	useStandaloneWorkspaceFixture(t)
	command := newCommand()
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	err := command.Run(context.Background(), []string{
		"aos",
		"--agent", "codex",
		"--role", "engineer",
		"--image", "aos:test",
		"--auth=false",
		"--dry-run",
		"--",
		"--version",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, want := range []string{
		"docker run",
		"--composed",
		"--guarded",
		"_container-acompose",
		"-- codex --version",
		substrateVolume,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("standalone dry run missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "ward agent") {
		t.Fatalf("standalone dry run invoked Ward:\n%s", rendered)
	}
}

func TestIntegratedStandaloneDryRunWithoutRepositoryPlanUsesCallerWorkspace(t *testing.T) {
	runtime := useStandaloneWorkspaceWithoutRepositoryPlanFixture(t)
	command := newCommandForInvocation("/usr/local/bin/aoscompose")
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	args := normalizeRoleShortcutArgs(commandDefaultsForInvocation("/usr/local/bin/aoscompose"), []string{
		"aoscompose",
		"--image", "aos:test",
		"--auth=false",
		"--dry-run",
		"engineer",
	})
	if err := command.Run(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, want := range []string{
		"source=" + runtime.CWD + ",target=/workspace",
		"--workdir /workspace",
		"--workspace /workspace",
		",target=" + defaultAgentHome,
		"--role engineer",
		"--layout codex",
		"-- codex",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("missing-plan standalone dry run missing %q:\n%s", want, rendered)
		}
	}
	_, lease := onlyNativeLease(t, runtime)
	if len(lease.Artifacts) != 0 {
		t.Fatalf("missing-plan launch created worktree artifacts: %#v", lease.Artifacts)
	}
	if lease.SessionHome == "" {
		t.Fatalf("missing-plan launch did not keep standalone home: %#v", lease)
	}
}

func TestIntegratedStandaloneDryRunUsesNativeShadowWorkspace(t *testing.T) {
	runtime, repository := useStandaloneWorkspaceFixture(t)
	docs := addNativeTestDirectory(t, repository, "docs")
	t.Chdir(docs)
	command := newCommandForInvocation("/usr/local/bin/aoscompose")
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	args := normalizeRoleShortcutArgs(commandDefaultsForInvocation("/usr/local/bin/aoscompose"), []string{
		"aoscompose",
		"--image", "aos:test",
		"--auth=false",
		"--dry-run",
		"engineer",
		"--version",
	})
	if err := command.Run(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, want := range []string{
		",target=/workspace",
		"--workdir /workspace/owner/one/docs",
		"--workspace /workspace/owner/one/docs",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("shadow workspace dry run missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "source="+docs) {
		t.Fatalf("standalone dry run mounted the caller subdirectory directly:\n%s", rendered)
	}
	_, lease := onlyNativeLease(t, runtime)
	if len(lease.Artifacts) != 1 || !strings.Contains(lease.Artifacts[0].Worktree, filepath.Join("projects", "owner", "one")) {
		t.Fatalf("lease artifacts = %#v", lease.Artifacts)
	}
}

func TestIntegratedStandaloneDryRunFromProjectsRootMountsFleetSurface(t *testing.T) {
	runtime, _ := useStandaloneWorkspaceFixture(t)
	t.Chdir(runtime.ProjectsRoot)
	command := newCommand()
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	if err := command.Run(context.Background(), []string{
		"aos",
		"--agent", "codex",
		"--role", "engineer",
		"--image", "aos:test",
		"--auth=false",
		"--dry-run",
		"--",
		"--version",
	}); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, want := range []string{
		",target=/workspace",
		"--workdir /workspace",
		"--workspace /workspace",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("projects-root dry run missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "source="+runtime.ProjectsRoot) {
		t.Fatalf("standalone dry run mounted canonical projects root directly:\n%s", rendered)
	}
	_, lease := onlyNativeLease(t, runtime)
	if len(lease.Artifacts) != 1 {
		t.Fatalf("lease artifacts = %#v", lease.Artifacts)
	}
	worktree := lease.Artifacts[0].Worktree
	if !strings.Contains(worktree, filepath.Join("projects", "owner", "one")) {
		t.Fatalf("lease artifact lost owner/repo hierarchy: %#v", lease.Artifacts[0])
	}
	if _, err := os.Stat(filepath.Join(worktree, "README.md")); err != nil {
		t.Fatalf("leased repository is unavailable: %v", err)
	}
}

func TestIntegratedStandaloneDryRunMountsSafeHomeProjection(t *testing.T) {
	runtime, _ := useStandaloneWorkspaceFixture(t)
	for _, path := range []string{
		filepath.Join(runtime.Home, ".agents"),
		filepath.Join(runtime.Home, ".claude"),
		filepath.Join(runtime.Home, ".aws"),
		filepath.Join(runtime.Home, ".codex"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(runtime.Home, ".agents", "settings.json"),
		filepath.Join(runtime.Home, ".claude", "settings.json"),
		filepath.Join(runtime.Home, ".claude", ".credentials.json"),
		filepath.Join(runtime.Home, ".aws", "config"),
		filepath.Join(runtime.Home, ".codex", "auth.json"),
		filepath.Join(runtime.Home, ".gitconfig"),
	} {
		if err := os.WriteFile(path, []byte("test\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	command := newCommand()
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	if err := command.Run(context.Background(), []string{
		"aos",
		"--agent", "codex",
		"--role", "engineer",
		"--image", "aos:test",
		"--auth=false",
		"--dry-run",
		"--",
		"--version",
	}); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	if !strings.Contains(rendered, ",target="+defaultAgentHome) {
		t.Fatalf("standalone dry run did not mount a projected HOME:\n%s", rendered)
	}
	if strings.Contains(rendered, "source="+runtime.Home) {
		t.Fatalf("standalone dry run mounted host HOME directly:\n%s", rendered)
	}
	if strings.Contains(rendered, "--tmpfs "+defaultAgentHome) {
		t.Fatalf("standalone dry run kept blank HOME tmpfs:\n%s", rendered)
	}
	_, lease := onlyNativeLease(t, runtime)
	if lease.SessionHome == "" {
		t.Fatalf("lease did not record a standalone home: %#v", lease)
	}
	for _, path := range []string{
		filepath.Join(lease.SessionHome, ".agents", "settings.json"),
		filepath.Join(lease.SessionHome, ".claude", "settings.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("safe home projection missing %s: %v", path, err)
		}
	}
	for _, path := range []string{
		filepath.Join(lease.SessionHome, ".claude", ".credentials.json"),
		filepath.Join(lease.SessionHome, ".aws", "config"),
		filepath.Join(lease.SessionHome, ".codex", "auth.json"),
		filepath.Join(lease.SessionHome, ".gitconfig"),
	} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("safe home projection included denied path %s: %v", path, err)
		}
	}
}

func TestIntegratedStandaloneCodexAuthFailurePrecedesDockerPlan(t *testing.T) {
	clearCodexAuthEnvironment(t)
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	if err := os.Mkdir(filepath.Join(codexHome, "auth.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	command := newCommand()
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	err := command.Run(context.Background(), []string{
		"aos",
		"--agent", "codex",
		"--role", "engineer",
		"--image", "aos:test",
		"--dry-run",
		"--",
		"exec", "probe",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported credential source") {
		t.Fatalf("missing Codex auth error = %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("missing Codex auth rendered a Docker plan:\n%s", output.String())
	}
}

func TestIntegratedStandaloneCompatibilityFlagsCannotDisableContexts(t *testing.T) {
	useStandaloneWorkspaceFixture(t)
	command := newCommand()
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	err := command.Run(context.Background(), []string{
		"aos",
		"--agent", "codex",
		"--role", "engineer",
		"--image", "aos:test",
		"--composed=false",
		"--guarded=false",
		"--auth=false",
		"--dry-run",
		"--",
		"--version",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, want := range []string{
		"docker run",
		"_container-acompose",
		"--composed",
		"--guarded",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("standalone dry run missing forced context %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "ward agent") {
		t.Fatalf("standalone composed dry run invoked Ward:\n%s", rendered)
	}
}

func TestAOSWardInvocationAlwaysUsesWardAndBothContexts(t *testing.T) {
	t.Parallel()
	command := newCommandForInvocation("/usr/local/bin/aosward-windows-amd64.exe")
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	err := command.Run(context.Background(), []string{
		"aosward",
		"--agent", "codex",
		"--role", "director",
		"--image", "aos:test",
		"--warded=false",
		"--composed=false",
		"--guarded=false",
		"--dry-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, want := range []string{
		"_container-context-bundle",
		"--composed",
		"--guarded",
		"ward agent director",
		"--context-bundle '<AOS_CONTEXT_BUNDLE>'",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("aosward dry run missing %q:\n%s", want, rendered)
		}
	}
}

func TestAOSComposeAliasesUseBothContextsWithoutWard(t *testing.T) {
	for _, alias := range []string{"aoscompose", "aoscomposed"} {
		alias := alias
		t.Run(alias, func(t *testing.T) {
			useStandaloneWorkspaceFixture(t)
			command := newCommandForInvocation("/usr/local/bin/" + alias + "-linux-amd64")
			var output bytes.Buffer
			command.Writer = &output
			command.ErrWriter = &output
			err := command.Run(context.Background(), []string{
				alias,
				"--agent", "codex",
				"--role", "engineer",
				"--image", "aos:test",
				"--composed=false",
				"--guarded=false",
				"--auth=false",
				"--dry-run",
				"--",
				"--version",
			})
			if err != nil {
				t.Fatal(err)
			}
			rendered := output.String()
			for _, want := range []string{
				"_container-acompose",
				"--network host",
				"--composed",
				"--guarded",
				"-- codex --version",
			} {
				if !strings.Contains(rendered, want) {
					t.Errorf("%s dry run missing %q:\n%s", alias, want, rendered)
				}
			}
			if strings.Contains(rendered, "ward agent") {
				t.Fatalf("%s dry run invoked Ward:\n%s", alias, rendered)
			}
		})
	}
}

func TestAOSComposeAliasesAcceptRoleShortcutWithDefaultAgent(t *testing.T) {
	for _, alias := range []string{"aoscompose", "aoscomposed"} {
		alias := alias
		t.Run(alias, func(t *testing.T) {
			useStandaloneWorkspaceFixture(t)
			command := newCommandForInvocation("/usr/local/bin/" + alias)
			var output bytes.Buffer
			command.Writer = &output
			command.ErrWriter = &output
			err := command.Run(context.Background(), []string{
				alias,
				"--image", "aos:test",
				"--auth=false",
				"--dry-run",
				"engineer",
			})
			if err != nil {
				t.Fatal(err)
			}
			rendered := output.String()
			for _, want := range []string{
				"_container-acompose",
				"--role engineer",
				"-- codex",
			} {
				if !strings.Contains(rendered, want) {
					t.Errorf("%s shortcut dry run missing %q:\n%s", alias, want, rendered)
				}
			}
		})
	}
}

func TestAOSComposeAliasRoleShortcutAcceptsPositionalHarnessOverride(t *testing.T) {
	useStandaloneWorkspaceFixture(t)
	command := newCommandForInvocation("/usr/local/bin/aoscompose")
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	args := normalizeRoleShortcutArgs(launchDefaults{RoleShortcut: true}, []string{
		"aoscompose",
		"--image", "aos:test",
		"--auth=false",
		"--dry-run",
		"engineer",
		"goose",
		"--version",
	})
	if err := command.Run(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, want := range []string{
		"_container-acompose",
		"--role engineer",
		"-- goose --version",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("shortcut override dry run missing %q:\n%s", want, rendered)
		}
	}
}

func TestNormalizeRoleShortcutArgsLeavesSubcommandsAlone(t *testing.T) {
	t.Parallel()
	args := []string{"aoscompose", "version"}
	got := normalizeRoleShortcutArgs(launchDefaults{RoleShortcut: true}, args)
	if strings.Join(got, "\x00") != strings.Join(args, "\x00") {
		t.Fatalf("normalizeRoleShortcutArgs(version) = %v, want %v", got, args)
	}
}

func TestIntegratedStandaloneKubeconfigDryRun(t *testing.T) {
	useStandaloneWorkspaceFixture(t)
	kubeconfig := writeTestKubeconfig(
		t,
		filepath.Join(t.TempDir(), "operator config", "cluster config.yaml"),
	)
	command := newCommand()
	var output bytes.Buffer
	command.Writer = &output
	command.ErrWriter = &output
	err := command.Run(context.Background(), []string{
		"aos",
		"--agent", "codex",
		"--role", "ops",
		"--image", "aos:test",
		"--composed",
		"--auth=false",
		"--kubeconfig", kubeconfig,
		"--dry-run",
		"--",
		"--version",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, want := range []string{
		"source=" + kubeconfig + ",target=" + containerKubeconfig + ",readonly",
		"KUBECONFIG=" + containerKubeconfig,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("standalone dry run missing %q:\n%s", want, rendered)
		}
	}
}
