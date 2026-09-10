# Batch job rules

## No parallelism for rate-limited batch jobs

When a batch job's primary failure mode is upstream rate limiting (`gh` secondary limits, mod.io, ElevenLabs, Bluesky, Reddit, GitHub Search), serial execution is the correct default. Don't reach for `xargs -P`, GNU parallel, or goroutines as a "go faster" lever. A parallel run that trips the rate limit midway is strictly worse than a slow run that finishes cleanly - it leaves partial state across both sides and is harder to resume. If real throughput pressure exists, fix it with batched/bulk endpoints (GraphQL multi-query, batch APIs) before reaching for concurrency.

## Ward wrapper rules

When about to run a privileged op against kai-server, AWS, or k8s, or when wrapping a new sub-CLI inside ward, read `~/projects/coilysiren/ward/AGENTS.md` for full rules.
