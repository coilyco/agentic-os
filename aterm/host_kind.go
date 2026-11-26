package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/urfave/cli/v3"
)

// hostKind is what this host is for, contract treehouse.defs.v1 host_kind. See
// .agents/skills/tooling-aterm-client/references/host-kind.md.
type hostKind string

const (
	hostPersonal  hostKind = "personal"
	hostEphemeral hostKind = "ephemeral"
	hostDurable   hostKind = "durable"

	hostKindFormat  = "aterm.host.v1"
	hostKindFile    = "host.json"
	hostKindFeature = "host-kind"
)

var hostKinds = []hostKind{hostPersonal, hostEphemeral, hostDurable}

func parseHostKind(value string) (hostKind, bool) {
	kind := hostKind(value)
	return kind, slices.Contains(hostKinds, kind)
}

// hostKindStore holds the kind and the file it persists in. An empty path keeps it in
// memory, as a daemon built by a test has.
type hostKindStore struct {
	mu       sync.Mutex
	path     string
	kind     hostKind
	watchers []func(hostKind)
}

type hostKindRecord struct {
	Format string `json:"format"`
	Kind   string `json:"kind"`
}

// load reads path. A missing, unreadable or unknown file reads as personal, the safe
// default, and the reason comes back for the log.
func (h *hostKindStore) load(path string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.path, h.kind = path, hostPersonal
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var record hostKindRecord
	if err := json.Unmarshal(raw, &record); err != nil || record.Format != hostKindFormat {
		return fmt.Errorf("%s is not an %s record", path, hostKindFormat)
	}
	kind, ok := parseHostKind(record.Kind)
	if !ok {
		return fmt.Errorf("%s names no host kind %q", path, record.Kind)
	}
	h.kind = kind
	return nil
}

func (h *hostKindStore) get() hostKind {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.kind == "" {
		return hostPersonal
	}
	return h.kind
}

// set writes the file first, so a kind that did not persist is never reported.
func (h *hostKindStore) set(kind hostKind) (changed bool, watchers []func(hostKind), err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.path != "" {
		raw, _ := json.Marshal(hostKindRecord{Format: hostKindFormat, Kind: string(kind)})
		if err := writeHostKind(h.path, append(raw, '\n')); err != nil {
			return false, nil, err
		}
	}
	changed = h.kind != kind && !(h.kind == "" && kind == hostPersonal)
	h.kind = kind
	return changed, slices.Clone(h.watchers), nil
}

func writeHostKind(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0o600)
}

// hostKind is this host's kind, personal until someone sets another. Any goroutine.
func (d *daemon) hostKind() hostKind { return d.host.get() }

// onHostKind calls fn with the current kind now and after each change, from the
// goroutine that made the change, with no daemon lock held. fn must not block.
func (d *daemon) onHostKind(fn func(hostKind)) {
	d.host.mu.Lock()
	d.host.watchers = append(d.host.watchers, fn)
	d.host.mu.Unlock()
	fn(d.hostKind())
}

// setHostKind persists kind, then tells the watchers and every sessions subscriber.
func (d *daemon) setHostKind(kind hostKind) error {
	changed, watchers, err := d.host.set(kind)
	if err != nil {
		return fmt.Errorf("could not store the host kind: %w", err)
	}
	if !changed {
		return nil
	}
	d.logf("host kind is now %s", kind)
	for _, fn := range watchers {
		fn(kind)
	}
	d.broadcast(frame{Type: "host_kind", HostKind: string(kind)})
	return nil
}

// hostKindFrame answers host_kind: a read without host_kind, a change with it. A change
// is judged as typing is, so a locked remote device or a seat's browser cannot make it.
func (d *daemon) hostKindFrame(cl *client, message frame) error {
	if message.HostKind != "" {
		kind, ok := parseHostKind(message.HostKind)
		if !ok {
			return withExit(exitUsage, fmt.Errorf("no host kind named %q, it is personal, ephemeral or durable", message.HostKind))
		}
		if cl.web || cl.remote {
			if err := d.typingRefusal(cl, "a process inside an aterm session cannot set the host kind from a browser, use `aterm host kind`"); err != nil {
				return err
			}
		}
		if err := d.setHostKind(kind); err != nil {
			return err
		}
	}
	return cl.c.write(frame{Type: "host_kind", ID: message.ID, HostKind: string(d.hostKind())})
}

func newHostCommand() *cli.Command {
	return &cli.Command{
		Name:  "host",
		Usage: "read or set facts about this host",
		Commands: []*cli.Command{{
			Name:      "kind",
			Usage:     "print this host's kind, or set it to personal, ephemeral or durable",
			ArgsUsage: "[personal|ephemeral|durable]",
			Description: "personal is a desktop or laptop a person uses and closes, ephemeral a batch machine\n" +
				"that ends when its work is done, durable an always-on server. A host never told its\n" +
				"kind is personal. The daemon keeps it across restarts and reports it to clients and\n" +
				"the Treehouse broker.",
			Action: func(_ context.Context, cmd *cli.Command) error {
				if cmd.Args().Len() > 1 {
					return withExit(exitUsage, errors.New("aterm host kind takes one kind or none"))
				}
				request := frame{Type: "host_kind"}
				if cmd.Args().Len() == 1 {
					kind, ok := parseHostKind(cmd.Args().First())
					if !ok {
						return withExit(exitUsage, fmt.Errorf("no host kind named %q, it is personal, ephemeral or durable", cmd.Args().First()))
					}
					request.HostKind = string(kind)
				}
				c, err := dialDaemon(true)
				if err != nil {
					return withExit(exitMissing, err)
				}
				defer c.Close()
				if !slices.Contains(c.features, hostKindFeature) {
					return errors.New("the running aterm daemon predates host kinds. " +
						"It restarts on the upgraded binary after five idle minutes")
				}
				reply, err := c.request(request)
				if err != nil {
					if reply.Code != 0 {
						return withExit(reply.Code, err)
					}
					return err
				}
				_, err = fmt.Fprintln(cmd.Root().Writer, reply.HostKind)
				return err
			},
		}},
	}
}
