package main

import (
	"context"
	"sync"
	"time"
)

// coordinatorTTL is how long a read roster answers, so each push does not shell out.
const coordinatorTTL = time.Minute

// coordinatorCache is the last roster read, kept for the push gate.
type coordinatorCache struct {
	mu     sync.Mutex
	at     time.Time
	roster listedRoster
	ok     bool
}

// mayPing says whether a role may reach Kai by an automatic push: only a coordinator.
// A failed read keeps the last good roster, and with none the answer is no.
func (d *daemon) mayPing(role string) bool {
	c := &d.coordinators
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ok || time.Since(c.at) >= coordinatorTTL {
		if roster, err := d.roster(context.Background()); err == nil {
			c.roster, c.at, c.ok = roster, time.Now(), true
		}
	}
	return c.ok && coordinatorRole(c.roster, role)
}
