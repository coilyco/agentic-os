package main

import "slices"

// claimPoolName settles a launch's session name with the daemon, starting one when
// none answers. Any failure answers base. See docs/aterm-daemon.md.
func claimPoolName(base string, peek bool) string {
	c, err := dialDaemon(!peek)
	if err != nil {
		return base
	}
	defer c.Close()
	if slices.Contains(c.features, claimFeature) {
		reply, err := c.request(frame{Type: "claim", Session: base, Peek: peek})
		if err != nil || reply.Session == "" {
			return base
		}
		return reply.Session
	}
	// A daemon predating claim idles out later, so this launch picks from the list.
	// Two launches at once can race here, and the loser's spawn is refused.
	reply, err := c.request(frame{Type: "list"})
	if err != nil {
		return base
	}
	live := map[string]bool{}
	for _, view := range reply.Sessions {
		live[view.Name] = true
	}
	return poolName(base, func(name string) bool { return live[name] })
}
