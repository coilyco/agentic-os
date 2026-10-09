package main

import (
	"context"
	"fmt"
	"slices"

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
				Usage: "print a one-time code that lets one device enroll a passkey",
				Action: func(_ context.Context, cmd *cli.Command) error {
					reply, err := passkeyAdmin("passkey_mint")
					if err != nil {
						return err
					}
					_, err = fmt.Fprintf(cmd.Root().Writer, "enrollment code %s, good for %d minutes and one enrollment.\n"+
						"Enter it in the aterm client at %s, or in the aterm Android app.\n", reply.EnrollCode, reply.ExpiresIn/60, passkeyOrigin)
					return err
				},
			},
			{
				Name:  "revoke",
				Usage: "forget every enrolled passkey, so no remote device types until one enrolls again",
				Action: func(_ context.Context, cmd *cli.Command) error {
					if _, err := passkeyAdmin("passkey_revoke"); err != nil {
						return err
					}
					_, err := fmt.Fprintln(cmd.Root().Writer, "every passkey is revoked")
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
