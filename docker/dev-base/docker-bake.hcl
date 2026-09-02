variable "TAG" {
  default = "dev-base-local"
}

variable "PLATFORM" {
  default = "linux/amd64"
}

# Space-separated rustup toolchains baked in addition to stable, so a repo
# pinning one in rust-toolchain.toml never fetches a channel at build time.
variable "RUST_PINNED_VERSIONS" {
  default = "1.90.0"
}

group "default" {
  targets = ["full"]
}

target "language" {
  context    = "docker/dev-base"
  dockerfile = "Dockerfile"
  platforms  = [PLATFORM]
  output     = ["type=cacheonly"]
  args = {
    RUST_PINNED_VERSIONS = RUST_PINNED_VERSIONS
  }
}

target "lang-node" {
  inherits = ["language"]
  target   = "dev-base-lang-node"
  tags     = ["agentic-os:lang-node-${TAG}"]
}

target "lang-go" {
  inherits = ["language"]
  target   = "dev-base-lang-go"
  tags     = ["agentic-os:lang-go-${TAG}"]
}

target "lang-dotnet" {
  inherits = ["language"]
  target   = "dev-base-lang-dotnet"
  tags     = ["agentic-os:lang-dotnet-${TAG}"]
}

target "lang-rust" {
  inherits = ["language"]
  target   = "dev-base-lang-rust"
  tags     = ["agentic-os:lang-rust-${TAG}"]
}

target "lang-python" {
  inherits = ["language"]
  target   = "dev-base-lang-python"
  tags     = ["agentic-os:lang-python-${TAG}"]
}

target "full" {
  context    = "docker/dev-base"
  dockerfile = "full/Dockerfile"
  target     = "dev-base-full"
  platforms  = [PLATFORM]
  tags       = ["agentic-os:${TAG}"]
  output     = ["type=docker"]
  args = {
    RUST_PINNED_VERSIONS = RUST_PINNED_VERSIONS
    BASE_IMAGE        = "agentic-os:lang-rust-${TAG}"
    LANG_NODE_IMAGE   = "agentic-os:lang-node-${TAG}"
    LANG_GO_IMAGE     = "agentic-os:lang-go-${TAG}"
    LANG_DOTNET_IMAGE = "agentic-os:lang-dotnet-${TAG}"
    LANG_PYTHON_IMAGE = "agentic-os:lang-python-${TAG}"
  }
  contexts = {
    "agentic-os:lang-node-${TAG}"   = "target:lang-node"
    "agentic-os:lang-rust-${TAG}"   = "target:lang-rust"
    "agentic-os:lang-go-${TAG}"     = "target:lang-go"
    "agentic-os:lang-dotnet-${TAG}" = "target:lang-dotnet"
    "agentic-os:lang-python-${TAG}" = "target:lang-python"
    aosguard-spec                     = ".umbra"
    aosguard-python                   = "agentic_os"
    repo-lists                        = "aos-cli/repositories"
  }
}
