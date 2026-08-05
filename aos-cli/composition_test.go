package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

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
	for _, want := range []string{
		"WARD_CODEX_MODEL=",
		"WARD_CODEX_REASONING_EFFORT=",
		"WARD_CODEX_VERBOSITY=",
	} {
		if !strings.Contains(strings.Join(plan.Environment, " "), want) {
			t.Fatalf("Ward environment %q does not contain %q", plan.Environment, want)
		}
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
	for _, want := range []string{
		"WARD_CODEX_MODEL=",
		"WARD_CODEX_REASONING_EFFORT=",
		"WARD_CODEX_VERBOSITY=",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Ward launch did not receive AOS-owned harness environment %q:\n%s", want, rendered)
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

func TestIntegratedStandaloneCodexAuthFailurePrecedesDockerPlan(t *testing.T) {
	clearCodexAuthEnvironment(t)
	t.Setenv("CODEX_HOME", t.TempDir())
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
	if err == nil || !strings.Contains(err.Error(), "file-backed credentials not found") {
		t.Fatalf("missing Codex auth error = %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("missing Codex auth rendered a Docker plan:\n%s", output.String())
	}
}

func TestIntegratedStandaloneCompatibilityFlagsCannotDisableContexts(t *testing.T) {
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
	t.Parallel()
	for _, alias := range []string{"aoscompose", "aoscomposed"} {
		alias := alias
		t.Run(alias, func(t *testing.T) {
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

func TestIntegratedStandaloneKubeconfigDryRun(t *testing.T) {
	t.Parallel()
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
