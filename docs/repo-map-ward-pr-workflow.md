# Ward PR workflow and director merge paths

Use this map when the issue is about PR lifecycle, director merge, or burn-down.

If the same repo also exists under `/substrate`, work in `/workspace/agentic-os`.

## Find the current surfaces

- `rg -n "pull-request-and-merge|director merge|WARD-OUTCOME|repos\\.workflow" .ward docs`
- `aguard ops forgejo pr --help`
- `ward agent director --help`

## First check

- `aguard ops forgejo issue view <owner> <repo> <issue>`

## Notes

- The issue thread carries the workflow and merge authorization.
- Use `aguard ops forgejo pr view <owner> <repo> <pr>` after the PR exists.
