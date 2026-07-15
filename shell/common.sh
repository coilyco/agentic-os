# shellcheck shell=bash
# Shared bash + zsh init (bash/zsh common subset). See docs/features-shell-secrets.md.

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

# Host shells read the bundle live from the checkout (file://): a pull applies
# immediately, and launch needs no pin, gitsync, or credential. See docs/ward-specs.md.
_siren_ward_config_ref() {
  local repo
  repo=$(_siren_aos_repo_root) || return 1
  # Native Windows ward.exe cannot stat an MSYS /x/... path; -m emits X:/...
  if command -v cygpath >/dev/null 2>&1; then
    repo=$(cygpath -m "$repo") || return 1
  fi
  printf 'file://%s/.ward' "$repo"
}

export WARD_CONFIG_REF="$(_siren_ward_config_ref)"

# Dev-base image for `ward agent` dispatch: point host shells at the moving
# release alias.
export WARD_AGENT_IMAGE="forgejo.coilysiren.me/coilyco-flight-deck/agentic-os"
export WARD_AGENT_TAG="release"

# Env + PATH are inherited, so run once per terminal tree: the exported guard is
# the "has this run in this terminal yet?" check. Aliases/functions always define.
if [ -z "${_SIREN_SHELL_ENV:-}" ]; then
  export _SIREN_SHELL_ENV=1

  # ward owns the whole workspace root (the security boundary, all orgs).
  export WARD_LOCKDOWN_ROOT="$HOME/projects"

  # Prepend $1 to PATH if it's a real dir and not already present.
  _siren_path_prepend() {
    case ":$PATH:" in
      *":$1:"*) ;;
      *) [ -d "$1" ] && PATH="$1:$PATH" ;;
    esac
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

pre-commit-aos-version-defined() {
  local version
  version=$(grep -E '^version = ' "$HOME/projects/coilyco-flight-deck/agentic-os/pyproject.toml" | head -1 | sed 's/^version = "\(.*\)"$/\1/')
  echo "$version"
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

# Report every repo whose pinned agentic-os hook rev lags the version agentic-os
# defines now. Tab columns: repo, pinned rev, current version.
pre-commit-all-aos-version-outdated() {
  local current config repo rev
  current=$(pre-commit-aos-version-defined)
  for config in */*/.pre-commit-config.yaml; do
    [ -f "$config" ] || continue
    repo="${config%/.pre-commit-config.yaml}"
    rev=$(yq -r '.repos[] | select(.repo | test("agentic-os$")) | .rev' "$config")
    [ -z "$rev" ] || [ "$rev" = "null" ] && continue
    [ "${rev#v}" = "$current" ] && continue
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
  # shellcheck disable=SC1091
  source "$HOME/projects/coilyco-flight-deck/agentic-os/shell/common.sh"
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

# --- In-process AWS SSM secret loader (memory only, never disk) ---
ssm-load() {
  local quiet=0
  if [ "$1" = "--quiet" ]; then
    quiet=1
    shift
  fi
  local profile="${1:-default}"
  local region="${2:-us-east-1}"
  local json count name value key
  json=$(AWS_PROFILE="$profile" AWS_REGION="$region" \
    aws ssm get-parameters-by-path --path "/" --recursive --with-decryption \
    --query 'Parameters[].{Name:Name,Value:Value}' --output json) || return 1
  while IFS="$(printf '\t')" read -r name value; do
    key=$(printf '%s' "${name#/}" | tr '/-' '__' | tr '[:lower:]' '[:upper:]')
    export "$key=$value"
  done <<EOF
$(printf '%s' "$json" | jq -r '.[] | [.Name, .Value] | @tsv')
EOF
  count=$(printf '%s' "$json" | jq 'length')
  [ "$quiet" -eq 1 ] || printf 'loaded %s SSM exports into env\n' "$count"
}

ssm-get() {
  local name="$1"
  local profile="${2:-default}"
  local region="${3:-us-east-1}"
  AWS_PROFILE="$profile" AWS_REGION="$region" \
    aws ssm get-parameter --name "$name" --with-decryption \
    --query 'Parameter.Value' --output text
}

# Auto-cd a fresh interactive shell landing at $HOME into the projects root,
# matching Warp's default new-tab directory.
case $- in
  *i*)
    if [ "$PWD" = "$HOME" ] && [ -d "$HOME/projects" ]; then
      cd "$HOME/projects"
    fi
    ;;
esac
