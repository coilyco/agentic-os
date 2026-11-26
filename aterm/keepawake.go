package main

import (
	"sync"
)

// keepAwakeEnv set to "off" takes no assertion at all, as the tests do.
const keepAwakeEnv = "ATERM_KEEP_AWAKE"

// awakeWhy is what the platform lock says it is for, where the platform shows one.
const awakeWhy = "a Treehouse seat is running"

// awakeLock is one OS sleep assertion from awake_<os>.go, which dies with the daemon.
// See tooling-aterm-client/references/keep-awake.md.
type awakeLock interface {
	alive() bool
	describe() string
	release()
}

// awakeState is what `aterm status` reports about the host's keep-awake.
type awakeState struct {
	Held   bool   `json:"held"`
	Detail string `json:"detail"`
}

// keepAwake holds an assertion while a personal host runs any seat (COI-3768).
type keepAwake struct {
	acquire  func(why string) (awakeLock, error)
	personal func() bool
	seats    func() int
	logf     func(string, ...any)
	poke     chan struct{}

	mu     sync.Mutex
	lock   awakeLock
	failed string
}

func newKeepAwake(acquire func(string) (awakeLock, error), personal func() bool, seats func() int, logf func(string, ...any)) *keepAwake {
	return &keepAwake{acquire: acquire, personal: personal, seats: seats, logf: logf, poke: make(chan struct{}, 1)}
}

// nudge asks for a settle after the seat count or host kind changed. It never blocks,
// so it is safe under d.mu.
func (k *keepAwake) nudge() {
	if k == nil {
		return
	}
	select {
	case k.poke <- struct{}{}:
	default:
	}
}

func (k *keepAwake) run(done <-chan struct{}) {
	k.settle()
	for {
		select {
		case <-done:
			k.drop()
			return
		case <-k.poke:
			k.settle()
		}
	}
}

// settle takes or lets go of the lock to match the host kind and seat count. A lock
// that died on its own counts as not held, so the next settle takes a new one.
func (k *keepAwake) settle() {
	want := k.personal() && k.seats() > 0
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.lock != nil && !k.lock.alive() {
		k.logf("keep-awake lock ended on its own: %s", k.lock.describe())
		k.lock = nil
	}
	switch {
	case want && k.lock == nil:
		lock, err := k.acquire(awakeWhy)
		if err != nil {
			if err.Error() != k.failed {
				k.logf("could not keep this host awake: %v", err)
			}
			k.failed = err.Error()
			return
		}
		k.lock, k.failed = lock, ""
		k.logf("keeping this host awake: %s", lock.describe())
	case !want && k.lock != nil:
		k.releaseLocked()
	case !want:
		k.failed = ""
	}
}

func (k *keepAwake) drop() {
	if k == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.lock != nil {
		k.releaseLocked()
	}
}

func (k *keepAwake) releaseLocked() {
	k.lock.release()
	k.logf("let this host sleep: no seat is running")
	k.lock = nil
}

func (k *keepAwake) state() *awakeState {
	if k == nil {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	switch {
	case k.lock != nil && k.lock.alive():
		return &awakeState{Held: true, Detail: k.lock.describe()}
	case k.failed != "":
		return &awakeState{Detail: "could not hold: " + k.failed}
	case !k.personal():
		return &awakeState{Detail: "not a personal host"}
	default:
		return &awakeState{Detail: "no seat is running"}
	}
}
