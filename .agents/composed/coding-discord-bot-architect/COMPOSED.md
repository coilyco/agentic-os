---
name: coding-discord-bot-architect
description: Build Discord bots in Discord.js or Pycord. Covers gateway intents,
  slash commands, interactive components, rate limiting, and sharding.
risk: unknown
source: vibeship-spawner-skills (Apache 2.0)
date_added: 2026-02-27
---

# Discord Bot Architect

Specialized skill for building production-ready Discord bots.
Covers Discord.js (JavaScript) and Pycord (Python), gateway intents,
slash commands, interactive components, rate limiting, and sharding.

## Rules

- Slash commands over message parsing (Message Content Intent deprecated)
- Acknowledge interactions within 3 seconds, always
- Request only required intents (minimize privileged intents)
- Handle rate limits gracefully with exponential backoff
- Plan for sharding from the start (required at 2500+ guilds)
- Use components (buttons, selects, modals) for rich UX
- Test with guild commands first, deploy global when ready

## Patterns

The full pattern catalogue (slash commands, interactive components, modal forms, gateway intents, rate limit handling, sharding, embeds, threading, voice connections, etc) lives in [`references/patterns.md`](references/patterns.md). Each entry covers the Discord.js (JavaScript) and Pycord (Python) shapes side by side.
## Sharp edges and operational concerns

Operational gotchas (acknowledge-immediately-process-later, gateway intent gating, deploy-script separation, testing workflow, secret hygiene, invite-URL generation, command deploy workflow, blocking-the-event-loop, reconnection handling, sharding at scale, modal timing) live in [`references/sharp-edges.md`](references/sharp-edges.md). Read when debugging or preparing a bot for production.
