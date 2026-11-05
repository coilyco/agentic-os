package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// nativeTrustPlan is what the launcher seeded for a Claude seat, kept so it can check the
// seed again just before it hands off, and so the launch leaves a record.
type nativeTrustPlan struct {
	Config   string
	Paths    []string
	External bool
}

// nativeTrustLogLimit bounds claude-trust.log. Past it a fresh file replaces it.
const nativeTrustLogLimit = 256 << 10

// logNativeClaudeTrust appends what was seeded and whether it held to claude-trust.log,
// so a dialog that still shows has a record to read (COI-3343).
func logNativeClaudeTrust(runtime nativeRuntime, stage string, plan nativeTrustPlan, result nativeSeedResult, err error) {
	path := nativeStatePath(runtime, "claude-trust.log")
	if info, statErr := os.Stat(path); statErr == nil && info.Size() > nativeTrustLogLimit {
		_ = os.Rename(path, path+".1")
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o700); mkErr != nil {
		return
	}
	file, openErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if openErr != nil {
		return
	}
	defer file.Close()
	failure := "-"
	if err != nil {
		failure = strings.ReplaceAll(err.Error(), "\n", " ")
	}
	fmt.Fprintf(file, "%s %s external=%t wrote=%t repairs=%d clean=%t paths=%s err=%s\n",
		runtime.Now.UTC().Format(time.RFC3339), stage, plan.External, result.Wrote, result.Repairs, result.Clean,
		strings.Join(plan.Paths, ","), failure)
}

// nativeTrustRecheckChecks is the one short read-back made just before the hand-off.
var nativeTrustRecheckChecks = []time.Duration{150 * time.Millisecond}

// recheckNativeClaudeTrust reads the seed back once more right before the exec, since the
// work between staging and the hand-off takes seconds. It never blocks a launch.
func recheckNativeClaudeTrust(runtime nativeRuntime, plan nativeTrustPlan) {
	if plan.Config == "" {
		return
	}
	result, err := ensureNativeClaudeTrust(plan.Config, plan.Paths, plan.External, nativeTrustRecheckChecks)
	logNativeClaudeTrust(runtime, "pre-exec", plan, result, err)
}
