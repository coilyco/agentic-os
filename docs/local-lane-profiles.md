# Local lane profiles

AOS projects a general role plus an explicit intent into one atomic,
model-opaque lane:

```text
aos --role community lane-default --intent knowledge-retrieval
```

The stable JSON output is:

```json
{
  "role": "community",
  "intent": "knowledge-retrieval",
  "harness": "sirens-echo",
  "route": "community/knowledge-retrieval"
}
```

The route is suitable for Agent Proxy's OpenAI-compatible `model` field. It is
control-plane data. An adapter must not append it to a system prompt, message,
personality source, or other model-visible content.

`harness-default` remains the compatibility surface and still prints only the
harness slug.

## Profile adapter

`--profile PATH` atomically updates an AOS-owned local profile while returning
the same four-field projection:

```text
aos --role community lane-default \
  --intent knowledge-retrieval \
  --profile ~/.config/aos/lanes/community.json
```

The file uses `agentic-os.local-lane-profile.v1`. Its `request.provider` is
`agent-proxy` and `request.model` is the logical route. Endpoint and credential
configuration remains operator-local or deployment-owned.

The adapter owns only `format`, `role`, `intent`, `harness`, `route`,
`request.provider`, and `request.model`. It preserves other top-level and
request fields, including user-owned prompt content. Repeating the same
projection is byte-idempotent. A malformed file, non-regular file, or file
carrying another format is preserved and rejected rather than overwritten.

## Boundary

* Agent Compose owns the general role and personality contract.
* AOS owns harness compatibility and this model-opaque projection.
* AOSH owns concrete model and fallback choices.
* Deploy owns Agent Proxy and LiteLLM runtime configuration.

AOS does not read AOSH and does not carry model, runtime, server, or fallback
data in its board, CLI result, or local profile.
