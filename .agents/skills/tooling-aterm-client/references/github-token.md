# aterm GitHub token and git credential helper

A seat pushes to coilyco repos as the coilyco-ops GitHub App, not as Kai's host login. The daemon holds the App key, so a seat never does. Source: `aterm/github_token.go` and `aterm/git_credential.go`. The daemon is described in [the aterm host daemon](../../../../docs/aterm-daemon.md).

## What a seat can run

**`aterm github-token --repo OWNER/REPO`** prints one installation token for that repo and nothing else. The daemon mints it from the App id and key in SSM, using its own AWS access. Only a live seat's own session token may ask, and a browser client never may. Only the `coilyco` owner is minted for.

**`--workflows` adds `workflows=write`**, which a push that changes `.github/workflows` needs. GitHub refuses that push on `contents=write` alone.

**`aterm git-credential <get|store|erase>`** is a git credential helper. On `get` for `https://github.com/coilyco/<repo>` it asks the daemon for a token with workflows and prints `username=x-access-token` and the password. `store` and `erase` do nothing, since a token lives an hour. Any other host, owner or protocol gets no answer, so git asks the next helper.

## Granted wide, minted narrow

Kai gave the App broad permissions on purpose, so no seat asks her for a grant. The installation is a ceiling. Each mint asks for less: one repo, about an hour, and by default contents write, pull requests write and metadata read. Only the git credential helper adds workflows, because that permission lets a token edit CI.

A GitHub 403 means the mint or the wiring is wrong. Widen what the mint asks for and never ask Kai. The daemon reads `ATERM_GITHUB_PERMISSIONS` as `name=read,name=write` to change the default. The same rule for the dispatch token is in infrastructure's `token-minting` skill, `references/github-app-dispatch-token.md`.

## Wiring it into git

Git sends the repo path to a helper only with `useHttpPath`, so scope both to the coilyco URL. The empty `helper =` line clears the inherited list, so the keychain helper never stores an App token. The last line is the host login as a fallback.

```
[credential "https://github.com/coilyco"]
	useHttpPath = true
	helper =
	helper = !aterm git-credential
	helper = !gh auth git-credential
```

**A failed mint never strands a push.** The helper prints why on stderr, exits non-zero, and git moves to the next helper. That fallback also hides an App problem behind the host login, so read stderr when a push is not credited to the App. Drop the last line once the App path has run clean.

**An older daemon** answers with a message that it predates the feature, and restarts on the upgraded binary after five idle minutes.

## Rollout

This repo authors the helper only. Writing the snippet into each host's gitconfig is infrastructure's ansible, never a hand edit.

## Checking it

`go test` in `aterm/` covers the helper protocol, the opt-in workflows permission, and the flag crossing the daemon frame. Git's own matching was run against a stand-in fallback helper, which showed the path scoped section reaches the helper, another owner never does, and a failure falls through. A real mint with workflows needs a daemon on the new binary.
