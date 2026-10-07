package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// The macOS per-user temp root put a Codex control socket past SUN_LEN, so new
// shadows use a short real directory in the shared temp parent (COI-2128).
const (
	nativeSharedTempRoot = "/private/tmp"
	// Codex fixes this tail under CODEX_HOME, so only the root is ours to shorten.
	nativeCodexSocketTail = "home/.codex/app-server-control/app-server-control.sock"
	// sun_path is 104 bytes on darwin and 108 on Linux, NUL included.
	nativeSocketLimitDarwin = 103
	nativeSocketLimitOther  = 107
)

// defaultNativeSessionsRoot is the root when AOS_NATIVE_SESSIONS_DIR is unset.
// Only macOS has the long temp root, so other hosts keep the existing path.
func defaultNativeSessionsRoot() (string, error) {
	if runtime.GOOS != "darwin" {
		return filepath.Join(ensureAOSTempAlias(), "native"), nil
	}
	return ensureNativePrivateSessionsRoot(nativeSharedTempRoot, os.Getuid())
}

// ensureNativePrivateSessionsRoot creates <base>/u<uid>/aos/native and refuses
// a level that is not a real 0700 directory this user owns.
func ensureNativePrivateSessionsRoot(base string, uid int) (string, error) {
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		return "", fmt.Errorf("native sessions base %s is not a usable directory: %v", base, err)
	}
	path := base
	for _, part := range []string{"u" + strconv.Itoa(uid), "aos", "native"} {
		path = filepath.Join(path, part)
		if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("create native sessions root %s: %w", path, err)
		}
		if err := verifyNativePrivateDir(path, uid); err != nil {
			return "", err
		}
	}
	return path, nil
}

func verifyNativePrivateDir(path string, uid int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect native sessions root %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("native sessions root %s is not a real directory (mode %s); "+
			"remove it or set AOS_NATIVE_SESSIONS_DIR", path, info.Mode())
	}
	owner, known := pathOwner(info)
	if !known || owner != uid {
		return fmt.Errorf("native sessions root %s is owned by uid %d, want %d; "+
			"remove it or set AOS_NATIVE_SESSIONS_DIR", path, owner, uid)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("native sessions root %s has mode %04o, want 0700; run chmod 700 %s",
			path, info.Mode().Perm(), path)
	}
	return nil
}

func nativeSocketLimit() int {
	if runtime.GOOS == "darwin" {
		return nativeSocketLimitDarwin
	}
	return nativeSocketLimitOther
}

// checkNativeCodexSocketBudget fails a Codex launch whose control socket would
// not bind. Codex resolves symlinks, so the longer spelling is the one measured.
func checkNativeCodexSocketBudget(sessionRoot string) error {
	measured := filepath.Join(sessionRoot, nativeCodexSocketTail)
	if parent, err := filepath.EvalSymlinks(filepath.Dir(sessionRoot)); err == nil {
		resolved := filepath.Join(parent, filepath.Base(sessionRoot), nativeCodexSocketTail)
		if len(resolved) > len(measured) {
			measured = resolved
		}
	}
	limit := nativeSocketLimit()
	if len(measured) <= limit {
		return nil
	}
	return fmt.Errorf("codex control socket %s is %d bytes, over the %d byte unix socket limit; "+
		"set AOS_NATIVE_SESSIONS_DIR to a shorter real directory", measured, len(measured), limit)
}
