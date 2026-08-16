# Warp default-shell layer

## Warp default-shell layer

On Windows, "which shell Warp launches for new tabs" (Settings > Features > Session, choosing among PowerShell / Git Bash / WSL / Cmd) is stored only in `warp.sqlite`. It has no `settings.toml` surface, so before this layer it drifted across machines and stayed invisible to `warp doctor`. The `warp/` module now manages it alongside the other SQLite keys (see [warp.md](warp.md)).

## Behaviour

- **apply** - resolves PowerShell 7 from disk and writes its path under the default-shell key in `warp.sqlite`. Idempotent: a converged value is left alone. Skips with a clear line when no PowerShell 7 is installed.
- **doctor** - reports drift when the live value differs from the resolved shell, fails when the key is absent, and NOTE-skips when PowerShell 7 is not installed.
- **macOS / Linux** - use the login shell and have no managed pref, so the layer is Windows-only and silently inert there.

## Public-safe path resolution

The desired shell is the path to the first PowerShell 7 binary found among the standard machine-class install locations (`C:\Program Files\PowerShell\7\pwsh.exe` and the `7-preview` sibling). Those segments carry no user-specific identity, so resolving at runtime - rather than hardcoding a machine-specific path into the repo - keeps the layer public-safe. The same candidate list backs `resolvePwshProfile` in `render.go`. Implementation lives in `warp/shell.go`; `HostPaths.DefaultShell` carries the resolved value.

## The inferred storage key

Warp does not document the `generic_string_objects` `storage_key` for this preference, and it can only be confirmed against a live Windows `warp.sqlite`. The key is set from a single constant (`defaultShellStorageKey`) in `warp/shell.go`. A wrong key is harmless - Warp ignores unknown rows - but it makes the layer a silent no-op rather than truly converging the UI setting. If a host ever shows the shell drifting in the Warp UI despite a green `apply`, dump the DB's `generic_string_objects` keys and reconcile the constant (and the value shape, if Warp stores more than a bare path).

See docs/warp.md for the original request.

## See also

- [warp.md](warp.md) - the warp module overview and the other state layers.
