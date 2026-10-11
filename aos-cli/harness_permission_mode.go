package main

import (
	"errors"
	"fmt"
)

// permissionModeSkip is the one value `permissions.<harness>` takes. An absent
// key launches as before. docs/native-harness-config.md
const permissionModeSkip = "skip"

const claudeSkipPermissionsFlag = "--dangerously-skip-permissions"

// validateHarnessPermissions admits only a harness with a skip flag aos knows.
// A guess at codex or goose spelling would launch a seat that ignores the pin.
func validateHarnessPermissions(permissions map[string]string) error {
	for harness, mode := range permissions {
		if harness != "claude" {
			return fmt.Errorf("harness launch profile permissions: %q has no skip mode aos knows", harness)
		}
		if mode != permissionModeSkip {
			return fmt.Errorf("harness launch profile permissions.%s is %q, want %q", harness, mode, permissionModeSkip)
		}
	}
	return nil
}

// applyHarnessPermissionMode inserts the skip flag after the harness. A typed
// --permission-mode or skip flag wins, as a typed --model does.
func applyHarnessPermissionMode(
	document harnessLaunchProfileDocument,
	command []string,
	harness string,
) []string {
	if document.Permissions[harness] != permissionModeSkip {
		return command
	}
	if nativeSpecArgsCarry(command, claudeSkipPermissionsFlag) || nativeSpecArgsCarry(command, "--permission-mode") {
		return command
	}
	return insertNativeHarnessArgs(command, harness, []string{claudeSkipPermissionsFlag})
}

// applyConfiguredPermissionMode is the launch-site call. It runs for every
// launch of the harness, role assigned or not, native and container alike.
func applyConfiguredPermissionMode(command []string, harness string) ([]string, error) {
	document, err := loadConfiguredHarnessLaunchProfiles()
	// Release builds embed the profiles, so only a bare dev build finds none.
	if errors.Is(err, errHarnessLaunchProfilesMissing) {
		return command, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s launch refused: %w", harness, err)
	}
	return applyHarnessPermissionMode(document, command, harness), nil
}
