package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
)

var version = "dev"

const (
	defaultImage    = "forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:release"
	defaultDensity  = "full"
	defaultDelivery = "native-skills"
)

func main() {
	cmd := &cli.Command{
		Name:    "aos",
		Usage:   "launch AOS runtime containers",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "role",
				Usage: "agent-compose role selected for the container",
			},
			&cli.StringFlag{
				Name:  "image",
				Value: defaultImage,
				Usage: "AOS dev-base image",
			},
			&cli.StringFlag{
				Name:  "layout",
				Usage: "agent-compose harness layout, inferred from the command when omitted",
			},
			&cli.StringFlag{
				Name:  "density",
				Value: defaultDensity,
				Usage: "agent-compose personality density: full or brief",
			},
			&cli.StringFlag{
				Name:  "delivery",
				Value: defaultDelivery,
				Usage: "agent-compose delivery mode: native-skills or compiled",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "print the Docker launch command without running it",
			},
			&cli.BoolFlag{
				Name:  "no-substrate",
				Usage: "skip materializing the baked read-only substrate",
			},
			&cli.BoolFlag{
				Name:  "auth",
				Value: true,
				Usage: "stage the selected harness's known host auth file when present",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "version",
				Usage: "print the build version",
				Action: func(_ context.Context, _ *cli.Command) error {
					fmt.Println(version)
					return nil
				},
			},
			{
				Name:   "harness-default",
				Usage:  "print the projected harness for a role-intent lane",
				Action: runHarnessDefault,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "intent",
						Usage: "model-opaque task intent to resolve",
					},
				},
			},
			{
				Name:      "acompose",
				Usage:     "launch a composed agent in the AOS image",
				ArgsUsage: "-- <harness> [args...]",
				Action:    runAcompose,
			},
			{
				Name:      "_container-acompose",
				Hidden:    true,
				ArgsUsage: "-- <harness> [args...]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "workspace",
						Required: true,
					},
					&cli.IntFlag{
						Name:     "uid",
						Required: true,
					},
					&cli.IntFlag{
						Name:     "gid",
						Required: true,
					},
				},
				Action: runContainerAcompose,
			},
		},
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "aos: %v\n", err)
		os.Exit(1)
	}
}

func runAcompose(ctx context.Context, cmd *cli.Command) error {
	role := strings.TrimSpace(cmd.String("role"))
	if role == "" {
		return fmt.Errorf("acompose needs --role")
	}
	command := argvAfterDash(os.Args)
	if len(command) == 0 {
		return fmt.Errorf("acompose needs a command after `--`")
	}
	cwd, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolve current working directory: %w", err)
	}
	layout, err := resolveLayout(cmd.String("layout"), command[0])
	if err != nil {
		return err
	}
	uid, gid := hostIdentity()
	authMounts := []authMount{}
	if cmd.Bool("auth") {
		authMounts = discoverAuthMounts(layout)
	}
	privateAccess, err := privateAccessForRole(role)
	if err != nil {
		return err
	}
	plan, err := buildLaunchPlan(launchOptions{
		Image:         cmd.String("image"),
		Role:          role,
		Layout:        layout,
		Density:       cmd.String("density"),
		Delivery:      cmd.String("delivery"),
		CWD:           cwd,
		Command:       command,
		UID:           uid,
		GID:           gid,
		TTY:           isTerminal(os.Stdin),
		NoSubstrate:   cmd.Bool("no-substrate"),
		AuthMounts:    authMounts,
		ForwardedEnvs: forwardedEnvironment(),
		PrivateAccess: privateAccess,
	})
	if err != nil {
		return err
	}
	if cmd.Bool("dry-run") {
		fmt.Fprintln(cmd.Root().Writer, shellJoin(append([]string{"docker"}, plan.DockerArgs...)))
		return nil
	}
	return runDocker(ctx, plan.DockerArgs)
}

func runContainerAcompose(ctx context.Context, cmd *cli.Command) error {
	if os.Getenv("AOS_CONTAINER") != "1" {
		return fmt.Errorf("_container-acompose is internal to an AOS container")
	}
	command := argvAfterDash(os.Args)
	if len(command) == 0 {
		return fmt.Errorf("_container-acompose needs a command after `--`")
	}
	role := strings.TrimSpace(cmd.String("role"))
	if role == "" {
		return fmt.Errorf("_container-acompose needs --role")
	}
	layout, err := resolveLayout(cmd.String("layout"), command[0])
	if err != nil {
		return err
	}
	spec, err := prepareContainer(ctx, bootstrapOptions{
		Role:        role,
		Layout:      layout,
		Density:     cmd.String("density"),
		Delivery:    cmd.String("delivery"),
		Workspace:   cmd.String("workspace"),
		UID:         cmd.Int("uid"),
		GID:         cmd.Int("gid"),
		Command:     command,
		NoSubstrate: cmd.Bool("no-substrate"),
	}, osCommandRunner{})
	if err != nil {
		return err
	}
	return execAs(cmd.Int("uid"), cmd.Int("gid"), spec)
}

func argvAfterDash(argv []string) []string {
	for i, arg := range argv {
		if arg == "--" {
			return append([]string(nil), argv[i+1:]...)
		}
	}
	return nil
}
