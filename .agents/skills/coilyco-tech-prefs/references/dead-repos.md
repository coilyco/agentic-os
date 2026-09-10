# Don't suggest dead or dormant repos

Before recommending an OSS project, library, tool, brew formula, plugin, or dependency, **verify it has had commits in the last 12 months**. Quick check:

```bash
curl -sL "https://api.github.com/repos/<owner>/<name>/commits?per_page=1" | grep '"date"'
```

No recent commits → don't surface it, or surface it explicitly framed as "this is dormant, here's why I'm flagging it anyway."

Applies to upstream libraries, dev tools, alternatives lists ("modern X replacements"), CLI helpers, browser extensions, anything actively recommended.

**Reason:** Kai has had Claude pitch her dead projects often enough to formalize the rule. The 12-month window is the bright line; project archived/maintenance-mode notices in the README count as dead regardless of last commit date.
