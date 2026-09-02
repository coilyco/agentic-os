# Guardfile headers

Why each `aosguard` guardfile is shaped the way it is. This lived in each
file's own header until the two-line comment cap reached YAML and KDL, and a
skill's `references/` takes no size cap, so it lands here whole.

## `actions.kdl`

AOSguard's repository-local Forgejo Actions resolvers and rerun bridges. These narrow Python modules share one command group and the coilyco-ops bot token.

## `aws.kdl`

aosguard ops aws - the AWS CLI surface as an allowlist (umbra execverb). Each grant reads like the invocation you'd type: `can run <service> <operation>`. A per-operation guard constrains the kwarg or positional that names the resource - the `--secret-id` value, the s3:// positional - with no env-var escape and no opaque gate. An operation not listed here is denied.

## `forgejo-admin.kdl`

Forgejo operations Forgejo refuses to the coilyco-ops bot, which is an org member holding push rather than an owner holding repo-admin. The separate wrapper keeps ordinary reads and writes on the bot token. Rationale: the tooling-aosguard skill.

## `forgejo-storage.kdl`

Application-aware Forgejo storage measurement. The generic kubectl surface keeps exec denied. This sealed bridge fixes every target and command in the packaged module, so callers cannot turn measurement into a remote shell.

## `forgejo.kdl`

Forgejo ops surface for the standalone AOSguard bundle. AOS authors it and AOSguard ships it. Ward mounts the built binary and owns nothing in here, and no leaf below defers a decision to Ward existing. Every `can` resolves its operationId by convention (verb + resource -> method + path); the only explicit `op` pins are on the repo-label denials below, where a bare `label` would resolve org-ward. Hardened per ward#109: orgs and repos lose their irreversible verbs, repo-label CRUD moves to org-labels, a cross-repo issue search and a move-issue action land, and every path-{owner} leaf is scoped to coily* owners. Repo-level label create/edit are additionally policy-disabled per ward#107 (dup priority-tier prevention). Auto-resolution per umbra#147. See the tooling-aosguard skill.

## `helm.kdl`

aosguard ops helm - Helm releases as an allowlist (umbra execverb, exec dialect). Wraps the image-baked `helm`, the other half of the cluster-write surface `kubectl.kdl` covers. Added for agentic-os#1502, which measured 40 helm mutation call sites across `coilyco-bridge/infrastructure` and `coilyco-bridge/deploy` and found 37 of them naming no cluster at all.

`--kube-context` is required on every call and allowlisted to the two clusters this estate runs, at wrap level so a later grant cannot forget it. Helm spells the selector `--kube-context` where kubectl spells it `--context`, which is the one place the two guardfiles cannot be read as copies of each other. The estate was not uniformly unpinned before this, it was inconsistently pinned - `deploy/scripts/deprovision-aws-ssm-mcp.sh` names the context on one line and omits it on the line above, against the same release - and a wrap-level `only pass` is what makes that particular inconsistency unrepresentable.

Every leaf additionally carries `deny-when kubeconfig matches "*"`, which `kubectl.kdl` does not yet. Allowlisting a context **name** pins a string rather than a target, because a kubeconfig context name is a local label that any file may define and point anywhere. The name guard and the file guard are only together a pinned cluster. Per-leaf rather than wrap-level because umbra refuses a wrap-level `deny-when` outside an `allow` list, by design and fail-closed. The matching gap on the kubectl surface is agentic-os#1506, deliberately left out of the helm change so a new surface is not held back by the risk of changing an old one.

Reads mount release state (`list`, `status`, `history`, `get`) and the two offline renderers (`template`, `lint`, `show`). Writes mount the friendly deploy surface (`upgrade`, `install`, `rollback`); `upgrade --install` is how every rollout in deploy actually invokes it. `uninstall` mounts, marked irreversible, so attended deprovision in coilyco-bridge/deploy is audited and cluster-pinned rather than reaching bare helm around the guard. It removes a release and everything it owns, and takes the history with it, so there is no rollback afterwards - the same class as `forgejo-admin package delete`. Its `delete` alias stays denied so the destructive spelling is the explicit one. `repo`, `registry`, `push`, `plugin`, `package`, and `create` stay denied because each changes where a chart comes from or runs code from one, which is a supply-chain decision rather than an operator verb. See the tooling-aosguard skill.

## `kubectl.kdl`

aosguard ops kubectl - host-native kubectl as an allowlist (umbra execverb, exec dialect). Wraps the image-baked `kubectl` (k3s over the tailnet). `exec` fixes the binary, never the cluster: this comment used to claim the target could not be substituted while `--context` reached kubectl unconstrained, so the wrap-level `only pass` below names the cluster on every call and allowlists it (#1348). Deny-by-default: only the read verbs (including `diff`, the non-mutating server-side preview of an apply) and the friendly deploy surface (apply / scale / rollout) mount. Destructive and shell-equivalent verbs (delete, drain/cordon, edit, patch, replace, exec, attach, cp, port-forward) are intentionally left unexposed - they fall to the host lockdown's bare-kubectl deny. Every grant audit-logs through the generated binary's own umbra audit path, which needs no other tool present. See the tooling-aosguard skill.

## `netlify.kdl`

aosguard ops netlify - the Netlify site surface as an allowlist (umbra execverb). The token resolves from SSM at exec time rather than living in a caller's environment, matching `aosguard ops actions`. Every leaf runs the packaged module, so a caller cannot reach the rest of the Netlify API.

`--site` is required on every leaf and allowlisted to the one site this estate owns. agentic-os#1349 closed a wrap that documented a fixed target while accepting any, and a new surface is where that class comes back.

Only the alias leaf writes, and it is read-modify-write: the API replaces `domain_aliases` wholesale, so sending one alias would delete the rest. It adds and removes in one call, because a rename split across two writes is two certificate events on a live site. See the tooling-aosguard skill.

## `redis.kdl`

aosguard ops redis - the Redis read surface as an allowlist (umbra execverb, exec dialect). Wraps redis-cli from redis-tools in the dev-base image.

The store this exists for is the shared mcp-beaver rate-limit bucket in coilyco-bridge/deploy services/mcp-ratelimit. Its whole content is a few small keys with TTLs, which is why `keys` is exposed at all: on a store this size the O(N) objection does not apply, and `scan` is here for the habit.

AUTH COMES FROM THE ENVIRONMENT, NEVER FROM ARGV. redis-cli reads REDISCLI_AUTH, and the two flags that would take a password on the command line - `-a` and `-u` - are absent from every allowlist below, so the guard rejects them. That is enforcement rather than convention: a flag not named in an `allow-flag` list is refused before the process runs.

WRITES ARE NOT HERE, AND THE REASON IS MONEY. A bucket key holds a spend budget, so `set` fabricates budget and `flushall` resets every budget at once. Neither is an agent verb. `del` is the one exception and is exposed deliberately: unsticking a single wedged bucket is ordinary operations, and it is bounded to one key at a time. `config set` stays denied because maxmemory-policy is load-bearing - moving it off noeviction turns an eviction into a silent budget reset, which is the failure the store exists to prevent.

## `signoz.kdl`

Every leaf fixes one exact tool on the pre-authenticated SigNoz MCP server selected by AOS convergence. Callers pass native MCP key=value arguments. They cannot replace the server, tool, config, or transport. Forgejo, kubectl, and server evidence stay in their own groups.

## `tailscale.kdl`

aosguard ops tailscale - the tailnet live-observe surface as an allowlist (umbra execverb, exec dialect). Wraps the image-baked `/usr/local/bin/tailscale` client so a live-observe surface can answer "is the tailnet up, is the peer reachable" itself instead of handing a human a runbook (agentic-os#447, the infrastructure#538 gap). Read-only by design: status/ping/netcheck and friends mount, while the state-changing verbs (up/down/login/logout/set/serve/funnel/ssh/file) stay denied - joining or reshaping the tailnet is ward's container bring-up axis, never an agent verb. AOSguard stays independent from Ward's fixed broker surface.

**The tailnet-wide device and tag inventory is here, not on an API leaf.** `status --json` carries `Tags`, `HostName`, `TailscaleIPs`, and `Online` for every peer, which answers "what tags does the tailnet actually report for this host" from the netmap the policy was evaluated against. agentic-os#1503 proposed promoting `list_tailscale_devices.py` from coilyco-bridge/infrastructure onto an API leaf; measuring first showed the local verb already answers it, so no leaf, no credential, and no new SSM parameter were added.

**There is deliberately no `acl` leaf, and there cannot be an agent-readable one.** Reading or rewriting the tailnet policy needs admin scope, and that credential is operator-held only by design: the ACL decides which machines an agent can SSH into, so its credential stays unreadable by the agents it governs (`coilyco-bridge/infrastructure` `docs/tailscale.md`). The `/tailscale/admin/oauth-client-{id,secret}` path some older docstrings still name is retired, and live SSM holds nothing under `/tailscale/` at all. A guarded ACL verb would have to put that credential in an agent-readable store, which inverts the boundary rather than guarding it.

## `telegram.kdl`

CI failure alerting as one verb, so no repository carries a copy of the alert program. The mapper holds the Telegram identity, so this leaf reaches a fixed cluster-local endpoint with no credential and takes no caller input: every field comes from the runner's own GITHUB_* environment. Sealed for the same reason `forgejo-storage measure` is - a caller cannot turn a fixed POST into an arbitrary one. See [`signoz.md`](signoz.md).
