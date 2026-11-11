package main

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/urfave/cli/v3"
)

func newPasskeyCommand() *cli.Command {
	return &cli.Command{
		Name:  "passkey",
		Usage: "enroll or revoke the passkey a remote tailnet device asserts to type",
		Description: "A device on the tailnet reads sessions but types only after a passkey assertion\n" +
			"with user verification, from the client at " + passkeyOrigin + ", or a device key\n" +
			"from the Android app. One code enrolls either, and revoke forgets both. Both verbs refuse\n" +
			"from inside a session, since only Kai's own terminal may change who can type.",
		Commands: []*cli.Command{
			{
				Name:  "enroll",
				Usage: "print a one-time code that lets one device enroll a passkey, then publish it for replicas",
				Flags: []cli.Flag{passkeysSSMFlag(), &cli.BoolFlag{Name: "no-publish", Usage: "print the code and return, without waiting to publish"}},
				Action: func(_ context.Context, cmd *cli.Command) error {
					before := storeStamp()
					reply, err := passkeyAdmin("passkey_mint")
					if err != nil {
						return err
					}
					out := cmd.Root().Writer
					fmt.Fprintf(out, "enrollment code %s, good for %d minutes and one enrollment.\n"+
						"Enter it in the aterm client at %s, or in the aterm Android app.\n", reply.EnrollCode, reply.ExpiresIn/60, passkeyOrigin)
					name := cmd.String("passkeys-ssm")
					if cmd.Bool("no-publish") || name == "off" {
						return nil
					}
					fmt.Fprintf(out, "Waiting for the enrollment, then publishing to %s for the other daemons. Ctrl-C skips it.\n", name)
					if !waitForEnrollment(before, time.Duration(reply.ExpiresIn)*time.Second) {
						fmt.Fprintln(out, "No enrollment arrived, so nothing was published.")
						return nil
					}
					if err := publishPasskeys(name); err != nil {
						return withExit(exitSpawn, fmt.Errorf("enrolled, but not published: %w", err))
					}
					_, err = fmt.Fprintf(out, "Published. Other daemons that follow %s pick it up on their next refresh.\n", name)
					return err
				},
			},
			{
				Name:  "publish",
				Usage: "publish this host's passkeys to SSM, where other daemons follow them",
				Flags: []cli.Flag{passkeysSSMFlag()},
				Action: func(_ context.Context, cmd *cli.Command) error {
					if err := publishPasskeys(cmd.String("passkeys-ssm")); err != nil {
						return withExit(exitSpawn, err)
					}
					_, err := fmt.Fprintf(cmd.Root().Writer, "published to %s\n", cmd.String("passkeys-ssm"))
					return err
				},
			},
			{
				Name:  "revoke",
				Usage: "forget every enrolled passkey, here and on the daemons that follow this one",
				Flags: []cli.Flag{passkeysSSMFlag(), &cli.BoolFlag{Name: "no-publish", Usage: "revoke here only, leaving the published passkeys"}},
				Action: func(_ context.Context, cmd *cli.Command) error {
					if _, err := passkeyAdmin("passkey_revoke"); err != nil {
						return err
					}
					out := cmd.Root().Writer
					fmt.Fprintln(out, "every passkey is revoked here")
					name := cmd.String("passkeys-ssm")
					if cmd.Bool("no-publish") || name == "off" {
						return nil
					}
					if err := publishPasskeys(name); err != nil {
						return withExit(exitSpawn, fmt.Errorf("revoked here, but the published passkeys remain: %w", err))
					}
					_, err := fmt.Fprintf(out, "Published the revoke to %s. Followers drop the passkeys on their next refresh.\n", name)
					return err
				},
			},
		},
	}
}

func passkeyAdmin(kind string) (frame, error) {
	c, err := dialDaemon(false)
	if err != nil {
		return frame{}, withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, passkeyFeature) {
		return frame{}, fmt.Errorf("the running aterm daemon predates passkeys. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	reply, err := c.request(frame{Type: kind})
	if err != nil && reply.Code != 0 {
		return reply, withExit(reply.Code, err)
	}
	return reply, err
}

func passkeysSSMFlag() cli.Flag {
	return &cli.StringFlag{
		Name:    "passkeys-ssm",
		Value:   passkeysSSMName,
		Usage:   "the SSM parameter other daemons follow, or off to publish nowhere",
		Sources: cli.EnvVars("ATERM_PASSKEYS_SSM"),
	}
}
