package main

import (
	"bytes"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeAwakeLock struct {
	dead     atomic.Bool
	released atomic.Int32
}

func (l *fakeAwakeLock) alive() bool      { return !l.dead.Load() }
func (l *fakeAwakeLock) describe() string { return "fake lock" }
func (l *fakeAwakeLock) release()         { l.released.Add(1); l.dead.Store(true) }

type fakeAwake struct {
	seats    atomic.Int32
	personal atomic.Bool
	fail     atomic.Bool
	locks    []*fakeAwakeLock
}

func (f *fakeAwake) keeper() *keepAwake {
	acquire := func(string) (awakeLock, error) {
		if f.fail.Load() {
			return nil, errors.New("no caffeinate")
		}
		lock := &fakeAwakeLock{}
		f.locks = append(f.locks, lock)
		return lock, nil
	}
	return newKeepAwake(acquire, f.personal.Load, func() int { return int(f.seats.Load()) }, func(string, ...any) {})
}

func (f *fakeAwake) held() int {
	count := 0
	for _, lock := range f.locks {
		if lock.alive() {
			count++
		}
	}
	return count
}

func TestKeepAwakeHoldsOneLockWhileAnySeatRuns(t *testing.T) {
	f := &fakeAwake{}
	f.personal.Store(true)
	k := f.keeper()
	k.settle()
	if len(f.locks) != 0 || k.state().Held || k.state().Detail != "no seat is running" {
		t.Fatalf("no seat should take no lock, got %d, %+v", len(f.locks), k.state())
	}
	f.seats.Store(1)
	k.settle()
	f.seats.Store(3)
	k.settle()
	if len(f.locks) != 1 || f.held() != 1 || !k.state().Held {
		t.Fatalf("seats should share one held lock, got %d taken, %d held", len(f.locks), f.held())
	}
	f.seats.Store(0)
	k.settle()
	if f.held() != 0 || f.locks[0].released.Load() != 1 || k.state().Held {
		t.Fatalf("the last seat ending should release the lock, %+v", k.state())
	}
}

func TestKeepAwakeTakesNoLockOffAPersonalHost(t *testing.T) {
	f := &fakeAwake{}
	f.seats.Store(2)
	k := f.keeper()
	k.settle()
	if len(f.locks) != 0 || k.state().Detail != "not a personal host" {
		t.Fatalf("a durable host should take no lock, got %d, %+v", len(f.locks), k.state())
	}
	f.personal.Store(true)
	k.settle()
	f.personal.Store(false)
	k.settle()
	if f.held() != 0 || len(f.locks) != 1 {
		t.Fatalf("turning durable with seats live should release, got %d held of %d", f.held(), len(f.locks))
	}
}

func TestKeepAwakeRetakesALockThatDiedAndReportsAFailure(t *testing.T) {
	f := &fakeAwake{}
	f.personal.Store(true)
	f.seats.Store(1)
	k := f.keeper()
	k.settle()
	f.locks[0].dead.Store(true)
	k.settle()
	if len(f.locks) != 2 || f.held() != 1 {
		t.Fatalf("a dead lock should be replaced, got %d taken, %d held", len(f.locks), f.held())
	}
	f.locks[1].dead.Store(true)
	f.fail.Store(true)
	k.settle()
	if state := k.state(); state.Held || !strings.Contains(state.Detail, "no caffeinate") {
		t.Fatalf("a failed take should say why, got %+v", state)
	}
}

func TestKeepAwakeRunSettlesOnNudgeAndReleasesAtStop(t *testing.T) {
	f := &fakeAwake{}
	f.personal.Store(true)
	k := f.keeper()
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() { k.run(done); close(stopped) }()
	f.seats.Store(1)
	k.nudge()
	deadline := time.Now().Add(2 * time.Second)
	for !k.state().Held {
		if time.Now().After(deadline) {
			t.Fatal("a nudge with a seat live should take the lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(done)
	<-stopped
	if f.held() != 0 {
		t.Fatal("stopping the daemon should release the lock")
	}
}

func TestKeepAwakeIsANoOpWhenOff(t *testing.T) {
	var k *keepAwake
	k.nudge()
	k.drop()
	if k.state() != nil {
		t.Fatal("an absent keep-awake reports nothing")
	}
	var out bytes.Buffer
	if err := writeStatus(&out, sessionStatus{Name: "s", Awake: &awakeState{Held: true, Detail: "fake lock"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "host awake: held, fake lock") {
		t.Fatalf("status should show the host's keep-awake:\n%s", out.String())
	}
}
