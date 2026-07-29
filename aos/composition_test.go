package main

import (
	"bytes"
	"context"
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
			Image: "aos:test", Role: "designer", Agent: "claude",
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
			name: "ward role",
			opts: integratedLaunchOptions{
				Image: "aos:test", Role: "designer", Agent: "codex", Warded: true,
			},
			want: "Ward ships director, qa, and engineer",
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
	want := "ward agent engineer owner/repo#267 --print --agent codex --image aos:test --context-bundle /cache/context --config agent.codex.model=gpt-5.6-terra --config agent.codex.effort=high --config agent.codex.verbosity=low"
	if got != want {
		t.Fatalf("Ward launch = %q, want %q", got, want)
	}
}

func TestIntegratedWardedDirectorCodexDryRunForwardsResolvedInputs(t *testing.T) {
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
	for _, want := range []string{
		"ward agent director",
		"--config agent.codex.model=gpt-5.6-sol",
		"--config agent.codex.effort=xhigh",
		"--config agent.codex.verbosity=low",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("dry run missing %q:\n%s", want, rendered)
		}
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

func TestIntegratedStandaloneDryRunUsesSelectedAgent(t *testing.T) {
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
		"--guarded",
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
		"--guarded",
		"_container-acompose",
		"-- codex --version",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("standalone dry run missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "ward agent") {
		t.Fatalf("standalone dry run invoked Ward:\n%s", rendered)
	}
	if strings.Contains(rendered, substrateVolume) {
		t.Fatalf("guard-only launch mounted the composition cache:\n%s", rendered)
	}
}
