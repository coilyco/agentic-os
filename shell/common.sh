# shellcheck shell=bash
# Shared bash + zsh init (bash/zsh common subset). See docs/install.md.

# shared-environment: begin
# Keep this block declarative. The rendered Windows PowerShell profile parses
# these export assignments directly so it does not need to launch Bash.
export ANSIBLE_FORCE_COLOR=1
export LANG=en_US.UTF-8
export EDITOR=code
export GIT_EDITOR=nano
export SSH_KEY_PATH="$HOME/.ssh/id_rsa"
export CLI_MFA=ykman
export AWS_PROFILE=default
export AWS_REGION=us-east-1
export AWS_PAGER=""
export BAT_PAGER=""
export HISTSIZE=100000
export SAVEHIST=100000
# Dev-base image for `ward agent` dispatch: point host shells at the moving
# release alias.
export WARD_AGENT_IMAGE="forgejo.coilysiren.me/coilyco-flight-deck/agentic-os"
export WARD_AGENT_TAG="release"
# shared-environment: end

# Bare names to clear, so the exports block above stays declarative. Both
# blocks are parsed by the rendered PowerShell profile. See docs/install.md.

# shared-environment-clear: begin
# Ward no longer consumes checkout-derived KDL references (ward#1615).
unset WARD_CONFIG_REF
# shared-environment-clear: end

_siren_aos_repo_root() {
  local repo source_dir
  for repo in "${AOS_REPO_ROOT:-}" \
    "${FORGEJO_WORKSPACE:-}" \
    "${GITHUB_WORKSPACE:-}" \
    /workspace/coilyco-flight-deck/agentic-os \
    "${BASH_SOURCE[0]:-}" \
    "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)" \
    /workspace/agentic-os \
    X:/projects/coilyco-flight-deck/agentic-os \
    /x/projects/coilyco-flight-deck/agentic-os \
    "$HOME/projects/coilyco-flight-deck/agentic-os"; do
    [ -n "$repo" ] || continue
    if [ "$repo" = "${BASH_SOURCE[0]:-}" ]; then
      source_dir="$(dirname "$(readlink "$repo" 2>/dev/null || printf '%s' "$repo")")"
      repo="$(cd "$source_dir/.." && pwd -P)"
    fi
    if git -C "$repo" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
      printf '%s\n' "$repo"
      return 0
    fi
  done
  return 1
}

_siren_projects_root() {
  local repo root
  if [ -n "${PROJECTS_ROOT:-}" ]; then
    root="$PROJECTS_ROOT"
  elif [ -d "$HOME/projects" ]; then
    root="$HOME/projects"
  else
    repo=$(_siren_aos_repo_root) || repo=""
    if [ -n "$repo" ]; then
      root="$(cd "$repo/../.." && pwd -P)"
    else
      root="$HOME/projects"
    fi
  fi
  # Native Windows tools need a drive-qualified value, while Git Bash accepts
  # the same mixed-slash form for shell paths.
  if command -v cygpath >/dev/null 2>&1; then
    root=$(cygpath -m "$root") || return 1
  fi
  printf '%s\n' "$root"
}

export PROJECTS_ROOT="$(_siren_projects_root)"

# Env + PATH are inherited, so run once per terminal tree: the exported guard is
# the "has this run in this terminal yet?" check. Aliases/functions always define.
_siren_fresh_terminal_tree=0
if [ -z "${_SIREN_SHELL_ENV:-}" ]; then
  _siren_fresh_terminal_tree=1
  export _SIREN_SHELL_ENV=1

  # ward owns the whole workspace root (the security boundary, all orgs).
  export WARD_LOCKDOWN_ROOT="$PROJECTS_ROOT"

  # Skipping when already present loses to `brew shellenv`, which re-prepends
  # unconditionally, so an inherited PATH keeps Homebrew ahead (aos#1467).
  _siren_path_prepend() {
    [ -d "$1" ] || return 0
    case ":$PATH:" in
      *":$1:"*)
        _sp=":$PATH:"
        while :; do
          case "$_sp" in
            *":$1:"*) _sp="${_sp%%":$1:"*}:${_sp#*":$1:"}" ;;
            *) break ;;
          esac
        done
        _sp="${_sp#:}"; PATH="${_sp%:}"; unset _sp ;;
    esac
    # An emptied PATH would take a trailing colon here, putting cwd on PATH.
    if [ -n "$PATH" ]; then PATH="$1:$PATH"; else PATH="$1"; fi
  }

  case "$(uname -s)" in
    Linux)
      if [ -d "$HOME/.pyenv" ]; then
        export PYENV_ROOT="$HOME/.pyenv"
        _siren_path_prepend "$PYENV_ROOT/bin"
        command -v pyenv >/dev/null && { eval "$(pyenv init --path)"; eval "$(pyenv init -)"; }
      fi
      [ -f "$HOME/.cargo/env" ] && . "$HOME/.cargo/env"
      # brew shellenv echoes a pre-set prefix rather than recomputing, so unset
      # first to stop a poisoned HOMEBREW_* (inherited value) perpetuating.
      unset HOMEBREW_PREFIX HOMEBREW_CELLAR HOMEBREW_REPOSITORY
      [ -x /home/linuxbrew/.linuxbrew/bin/brew ] && eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
      # A stray empty $HOMEBREW_REPOSITORY/Cellar hijacks HOMEBREW_CELLAR box-wide,
      # so rmdir it and recompute shellenv from the real prefix (agentic-os#325).
      if [ -n "${HOMEBREW_PREFIX:-}" ] && [ -n "${HOMEBREW_REPOSITORY:-}" ] && \
        [ "$HOMEBREW_PREFIX" != "$HOMEBREW_REPOSITORY" ] && \
        [ -d "$HOMEBREW_REPOSITORY/Cellar" ] && \
        [ -z "$(ls -A "$HOMEBREW_REPOSITORY/Cellar" 2>/dev/null)" ] && \
        [ -d "$HOMEBREW_PREFIX/Cellar" ]; then
        rmdir "$HOMEBREW_REPOSITORY/Cellar" 2>/dev/null
        unset HOMEBREW_PREFIX HOMEBREW_CELLAR HOMEBREW_REPOSITORY
        [ -x /home/linuxbrew/.linuxbrew/bin/brew ] && eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
      fi
      _siren_path_prepend "$HOME/.local/bin"
      _siren_path_prepend "$HOME/bin"
      # npm global prefix (~/.npmrc sets prefix=~/.npm-global); holds the claude CLI.
      _siren_path_prepend "$HOME/.npm-global/bin"
      export NVM_DIR="$HOME/.nvm"
      [ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
      alias ls='ls --color=auto'
      alias grep='grep --color=auto'
      ;;
    Darwin)
      # Unset first so brew recomputes rather than echoing a poisoned prefix.
      unset HOMEBREW_PREFIX HOMEBREW_CELLAR HOMEBREW_REPOSITORY
      [ -x /opt/homebrew/bin/brew ] && eval "$(/opt/homebrew/bin/brew shellenv)"
      for _d in /usr/local/share/dotnet "$HOME/.fabro/bin" /opt/homebrew/opt/gradle@7/bin \
        /opt/homebrew/opt/openjdk@17/bin "$HOME/.gem/ruby/3.4.0/bin" /opt/homebrew/opt/ruby/bin \
        "$HOME/.cargo/bin" "$HOME/.pyenv/shims" "$HOME/.local/bin" "$HOME/bin"; do
        _siren_path_prepend "$_d"
      done
      unset _d
      export JAVA_HOME=/opt/homebrew/opt/openjdk@17
      _crt="$HOME/Library/Application Support/Caddy/pki/authorities/local/root.crt"
      [ -r "$_crt" ] && export NODE_EXTRA_CA_CERTS="$_crt"
      unset _crt
      alias ls='ls -G'
      ;;
    MINGW*|MSYS*)
      for _d in "/c/Program Files/Git/usr/bin" "/c/Program Files/Git/bin" \
        "$HOME/.cargo/bin" "$HOME/.local/bin" "$HOME/bin"; do
        _siren_path_prepend "$_d"
      done
      unset _d
      # Stop msys rewriting leading-slash args before native exes (mangles SSM names).
      export MSYS_NO_PATHCONV=1
      ;;
  esac
  export PATH

  # Shared host-local overrides (machine-specific env, never tracked).
  [ -f "$HOME/.shellrc.local" ] && . "$HOME/.shellrc.local"
fi

# --- Aliases (per shell, always defined) ---
alias ..='cd ..'
alias ...='cd ../..'
alias ....='cd ../../..'
alias gt='git status'
alias gush='git push -u origin HEAD'
alias ll='ls -alF'
alias la='ls -A'
alias l='ls -CF'
alias ansible="ANSIBLE_FORCE_COLOR=1 uv tool run --from ansible-core ansible"
alias ansible-playbook-sync="ANSIBLE_FORCE_COLOR=1 uv tool run --from ansible-core ansible-playbook ./ansible/playbooks/sync.yml"

# --- Functions (per shell, always defined) ---
unalias bat 2>/dev/null || true
bat() {
  command bat --no-pager "$@"
}

# AOS gives native agents one leased, fleet-shaped worktree workspace and cleans
# dead predecessors before agent-compose converges the real harness.
_siren_native_shadow_available() {
  if [ -z "${_SIREN_NATIVE_SHADOW_PROBED+x}" ]; then
    _SIREN_NATIVE_SHADOW_PROBED=1
    if command -v aos >/dev/null 2>&1 &&
      command aos _native-shadow --probe >/dev/null 2>&1; then
      _SIREN_NATIVE_SHADOW_AVAILABLE=1
    else
      _SIREN_NATIVE_SHADOW_AVAILABLE=0
    fi
  fi
  [ "$_SIREN_NATIVE_SHADOW_AVAILABLE" = 1 ]
}

_siren_agent_launch() {
  local cli="$1"
  shift
  if command -v agent-compose >/dev/null 2>&1; then
    if _siren_native_shadow_available; then
      command aos _native-shadow --harness "$cli" -- agent-compose compose -- "$cli" "$@"
      return
    fi
    command agent-compose compose -- "$cli" "$@"
    return
  fi
  if _siren_native_shadow_available; then
    command aos _native-shadow --harness "$cli" -- "$cli" "$@"
    return
  fi
  command "$cli" "$@"
}

acompose() {
  local role="${1:-}"
  local harness="${2:-}"
  case "$role" in
    ""|-*) ;;
    *)
      # A flag in second place is a harness argument, never a seat.
      case "$harness" in
        -*) harness="" ;;
      esac
      if [ -n "$harness" ]; then
        shift 2
      else
        # No seat typed, so take the role's configured agent. aos owns the
        # launch profiles, so the shell asks rather than parsing them.
        harness=$(command aos _launch-agent "$role" 2>/dev/null) || harness=""
        [ -n "$harness" ] && shift 1
      fi
      # The seat is handed on unchecked: agent-compose owns the launchable set
      # and already refuses an unsupported one by name.
      if [ -n "$harness" ] && command -v agent-compose >/dev/null 2>&1; then
        if _siren_native_shadow_available; then
          # This file is symlinked live while aos ships on the release train,
          # so an aos too old to scope projection takes --role and ignores it.
          command aos _native-shadow --harness "$harness" --role "$role" \
            --assigned-role -- agent-compose launch "$role" "$harness" "$@"
          return
        fi
        command agent-compose launch "$role" "$harness" "$@"
        return
      fi
      ;;
  esac
  if command -v agent-compose >/dev/null 2>&1; then
    command agent-compose compose "$@"
    return
  fi
  command acompose "$@"
}

# aterm completes role slugs and seats from the live Agent Compose roster, so a
# slug that turned over stops completing. zsh calls this after compinit.
_siren_aterm_completion() {
  local shell_name="${1:-}"
  case "$shell_name" in
    bash | zsh) ;;
    *) return 0 ;;
  esac
  command -v aterm >/dev/null 2>&1 || return 0
  local script
  script="$(command aterm completion "$shell_name" 2>/dev/null)" || return 0
  [ -n "$script" ] || return 0
  eval "$script"
}

claude() { _siren_agent_launch claude "$@"; }
codex() { _siren_agent_launch codex "$@"; }
goose() { _siren_agent_launch goose "$@"; }
opencode() { _siren_agent_launch opencode "$@"; }

pre-commit-aos-version-defined() {
  local repo
  repo=$(_siren_aos_repo_root) || return 1
  git -C "$repo" tag --list 'aos-precommit-v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname |
    grep -E '^aos-precommit-v[0-9]+\.[0-9]+\.[0-9]+$' |
    head -1
}

pre-commit-aos-version-used() {
  yq -r '.repos[] | select(.repo | test("agentic-os$")) | .rev' \
    "${HOME}/projects/coilyco-flight-deck/agentic-os/.pre-commit-hooks.yaml"
}

pre-commit-hooks-used() {
  yq -r '.repos[] | select(.repo | test("agentic-os$")) | .hooks[].id' \
    "${HOME}/projects/coilyco-${1}/.pre-commit-config.yaml"
}

pre-commit-hooks-defined() {
  yq -r '.[].id' \
    "${HOME}/projects/coilyco-flight-deck/agentic-os/.pre-commit-hooks.yaml"
}

pre-commit-hooks-missing() {
  comm -23 <(pre-commit-hooks-used "${1}"| sort) <(pre-commit-hooks-defined | sort)
}

pre-commit-all-hooks-missing() {
  local config repo missing
  for config in */*/.pre-commit-config.yaml; do
    [ -f "$config" ] || continue
    repo="${config%/.pre-commit-config.yaml}"
    missing=$(comm -23 \
      <(yq -r '.repos[] | select(.repo | test("agentic-os$")) | .hooks[].id' "$config" | sort) \
      <(pre-commit-hooks-defined | sort))
    [ -z "$missing" ] && continue
    printf '==> %s\n%s\n' "$repo" "$missing"
  done
}

# Report every repo whose pinned agentic-os hook rev lags the latest fetched
# aos-precommit release tag. Tab columns: repo, pinned rev, current tag.
pre-commit-all-aos-version-outdated() {
  local current config repo rev
  current=$(pre-commit-aos-version-defined) || return 1
  [ -n "$current" ] || return 1
  for config in */*/.pre-commit-config.yaml; do
    [ -f "$config" ] || continue
    repo="${config%/.pre-commit-config.yaml}"
    rev=$(yq -r '.repos[] | select(.repo | test("agentic-os$")) | .rev' "$config")
    [ -z "$rev" ] || [ "$rev" = "null" ] && continue
    [ "$rev" = "$current" ] && continue
    printf '%s\t%s\t%s\n' "$repo" "$rev" "$current"
  done
}

git-default-branch() {
  git symbolic-ref --short refs/remotes/origin/HEAD | sed 's|^origin/||'
}

git-all-branches() {
  local repo branch
  for repo in coily*/*/.git; do
    [ -d "$repo" ] || continue
    repo="${repo%/.git}"
    git -C "$repo" for-each-ref --format='%(refname:short)' refs/heads |
      while IFS= read -r branch; do
        [ "$branch" = "main" ] && continue
        printf '%s\t%s\n' "$repo" "$branch"
      done
  done
}

git-all-stashes() {
  local repo stash
  for repo in coily*/*/.git; do
    [ -d "$repo" ] || continue
    repo="${repo%/.git}"
    git -C "$repo" stash list |
      while IFS= read -r stash; do
        [ -z "$stash" ] && continue
        printf '%s\t%s\n' "$repo" "$stash"
      done
  done
}

git-all-pull-main() {
  local repo branch rc=0
  for repo in coily*/*/.git; do
    [ -d "$repo" ] || continue
    repo="${repo%/.git}"
    printf '==> %s\n' "$repo"
    branch=$(git -C "$repo" branch --show-current) || continue
    if ! git -C "$repo" switch main; then
      rc=1
      continue
    fi
    if ! git -C "$repo" pull --ff-only; then
      rc=1
    fi
    if [ -n "$branch" ] && [ "$branch" != "main" ]; then
      git -C "$repo" switch "$branch" || rc=1
    fi
  done
  return "$rc"
}

git-all-dirty() {
  local limit="${1:-20}" repo uncommitted untracked
  for repo in coily*/*/.git; do
    [ -d "$repo" ] || continue
    repo="${repo%/.git}"
    uncommitted=$(git -C "$repo" status --porcelain | awk '$1 != "??" { print }' | head -n "$limit")
    untracked=$(git -C "$repo" status --porcelain | awk '$1 == "??" { print }' | head -n "$limit")
    [ -z "$uncommitted" ] && [ -z "$untracked" ] && continue
    printf '==> %s\n' "$repo"
    if [ -n "$uncommitted" ]; then
      printf 'uncommitted (first %s):\n%s\n' "$limit" "$uncommitted"
    fi
    if [ -n "$untracked" ]; then
      printf 'untracked (first %s):\n%s\n' "$limit" "$untracked"
    fi
  done
}

git-pr-title() {
  PAGER="" gh pr view --json title --jq ".title"
}

apply-aos-common() {
  local repo
  repo=$(_siren_aos_repo_root) || return 1
  # shellcheck disable=SC1091
  source "$repo/shell/common.sh"
}

# Exec from a WSL shell into native Windows PowerShell: SSH-into-WSL sessions skip
# the interop PATH injection, so re-add the Windows dirs, resolve pwsh, then exec.
wsl_to_native() {
  local d exe
  for d in /mnt/c/Windows/System32 /mnt/c/Windows \
    /mnt/c/Windows/System32/WindowsPowerShell/v1.0 \
    /mnt/c/Users/*/AppData/Local/Microsoft/WindowsApps; do
    case ":$PATH:" in
      *":$d:"*) ;;
      *) [ -d "$d" ] && PATH="$PATH:$d" ;;
    esac
  done
  export PATH
  exe=$(command -v pwsh.exe 2>/dev/null) \
    || exe=$(command -v powershell.exe 2>/dev/null) \
    || exe="/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe"
  exec "$exe" "$@"
}

git-merge-default-branch() {
  local default
  default=$(git-default-branch) || return 1
  git switch "$default" || return 1
  git pull
  git switch -
  git fetch origin "$default"
  git merge "origin/$default"
}

git-checkpoint() {
  local stamp="checkpoint-$(whoami)-$(date +%s)"
  local final="${1:-$stamp}"
  git commit . -m "$final" --allow-empty
  git push -u origin HEAD
}

git-squash() {
  git-merge-default-branch || return 1
  local default branch base
  default=$(git-default-branch) || return 1
  branch=$(git branch --show-current)
  base=$(git merge-base "$default" "$branch")
  git reset "$base"
  git add -A
  git commit . -m "$(git-pr-title)"
  git push -u origin HEAD -f
}

gm() {
  git commit -a -m "$1"
}

gt-conflicts() {
  git ls-files --unmerged --deduplicate | awk '{print $4}' | sort -u
}

docker-bash() {
  local id
  id=$(docker container ls --filter "name=$1" --quiet)
  docker exec -it "$id" bash
}

rg-code() {
  rg "$1" --files-with-matches | xargs -n1 code
}

# Pull every git repo one level down, returning to the start dir.
pull-all-repos() {
  local start_dir="$PWD" d
  for d in */; do
    if [ -d "$d.git" ]; then
      printf '==> %s\n' "${d%/}"
      git -C "$d" pull
    fi
  done
  cd "$start_dir"
}

count-lines() {
  rg --files | while read -r f; do
    printf '%s\t%s\n' "$(wc -l < "$f")" "$f"
  done | sort -rn
}

# bat every file two levels down: <dir>/*/*
bat-dir() {
  bat "${1:-.}"/*/*
}

# bat every file under a tree, flat. Skips dirs so bat never errors.
bat-tree() {
  tree -fi --noreport "${1:-.}" | while IFS= read -r f; do
    [ -f "$f" ] && printf '%s\0' "$f"
  done | xargs -0 bat
}

# Lazy: call when something needs the token, not at shell start.
github-token-load() {
  GITHUB_PERSONAL_ACCESS_TOKEN=$(gh auth token)
  export GITHUB_PERSONAL_ACCESS_TOKEN
  export HOMEBREW_GITHUB_PACKAGES_USER=coilysiren
  export HOMEBREW_GITHUB_PACKAGES_TOKEN="$GITHUB_PERSONAL_ACCESS_TOKEN"
}

# Delegates to scripts/ssm-get so the shell and every script share one
# implementation. See docs/install.md.
ssm-get() {
  local script
  for script in "$HOME/.local/bin/ssm-get" "$(_siren_aos_repo_root)/scripts/ssm-get"; do
    [ -x "$script" ] && { "$script" "$@"; return; }
  done
  echo "ssm-get: scripts/ssm-get not found; run 'just apply-shell-links'" >&2
  return 1
}

# Auto-cd a first shell from $HOME or an AOS native session root. Alacritty's
# new-window action otherwise copies the foreground agent's transient cwd.
case $- in
  *i*)
    _siren_startup_dir="${AOS_STARTUP_DIR:-$PROJECTS_ROOT}"
    _siren_startup_from_native=0
    if [ "$_siren_fresh_terminal_tree" = 1 ]; then
      _siren_native_sessions_dir="${AOS_NATIVE_SESSIONS_DIR:-${TMPDIR:-/tmp}/aos/native}"
      if [ -d "$_siren_native_sessions_dir" ]; then
        _siren_native_sessions_dir="$(cd "$_siren_native_sessions_dir" && pwd -P)"
        _siren_current_dir="$(pwd -P)"
        if [ "${_siren_current_dir##*/}" = projects ]; then
          _siren_native_parent="$(cd "$_siren_current_dir/../.." && pwd -P)"
          if [ "$_siren_native_parent" = "$_siren_native_sessions_dir" ]; then
            _siren_startup_from_native=1
          fi
        fi
      fi
    fi
    if [ "$PWD" = "$HOME" ] || [ "$_siren_startup_from_native" = 1 ]; then
      if [ -d "$_siren_startup_dir" ]; then
        cd "$_siren_startup_dir"
      fi
    fi
    unset _siren_startup_dir _siren_startup_from_native
    unset _siren_native_sessions_dir _siren_current_dir _siren_native_parent
    ;;
esac
unset _siren_fresh_terminal_tree
