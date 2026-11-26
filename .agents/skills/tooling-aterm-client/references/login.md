# Signing in a Claude seat

A Claude seat started with no login draws its sign-in as terminal text. The client reads those screens and stands a native card over them (COI-3678). The sign-in page opens in the device's own browser and the person pastes back the code it shows (COI-3679). The streamed Chromium is not used, since Cloudflare challenges a headless browser and the stream gave no way past it.

**The three screens, read from Claude Code v2.1.327** (captured at 134 columns, `login.test.ts` holds them with the one-use state and challenge blanked one character for one):

* **Method menu.** `Select login method:` then `❯ 1. Claude account with subscription`, `2.` Console, `3.` third-party. It has no `Enter to` footer, so `detectChoice` never reads it.
* **Link and code field.** `Browser didn't open? Use the URL below`, the authorize link wrapped at the terminal's width with nothing added, `Paste code here if prompted`, a box, `Esc to go back`. The link is `https://claude.com/cai/oauth/authorize` and its redirect is the hosted `platform.claude.com/oauth/code/callback`, so the page the person lands on shows the code. That is why a phone can finish a login that a seat's own localhost callback could not.
* **Refused code.** `OAuth error: That code wasn't accepted...` then `Press Enter to retry. (Esc to continue without logging in)`. Enter returns to the method menu with a new link. No credential is written.

**What the card does** (`login.ts` reads, `LoginCard.svelte` draws, `SessionPanel.svelte` presses keys):

* **Sign in with Claude** presses the arrows to the subscription row and Enter, and the link opens by `openExternal` the moment it shows. One tap, no second button. It is the opener of [android-opener.md](android-opener.md), not the `systembrowser` plugin, since that plugin is limited to the accounts address and exists for a return redirect that this flow does not have.
* **The field** sends exactly the pasted text, then Enter, 150ms apart. Only edge whitespace is dropped, since a phone keyboard adds a trailing space. An empty field sends nothing.
* **A refused code** shows the seat's own words on the card with Try again (Enter) and Continue without signing in (Esc).
* **The terminal stays reachable.** The card's Terminal button folds it to one line. The composer holds its send while the card is up, as it does for a menu.
* **The link is checked before it opens.** A seat prints whatever it likes, so only https, port-less, credential-less addresses on `claude.com`, `claude.ai`, `platform.claude.com` or `console.anthropic.com` ending in `/oauth/authorize` open. Any other link leaves the card with the field and a line saying so.

## Checked and not checked

* **Checked:** `login.test.ts` against the captures, a hand-wrapped narrow menu, and the Mock host seat's frames at 34 to 120 columns. `just aterm-client-drive` walks the Mock host seat Heron-Ox at 1280px and 320px: the card, the terminal under it, one tap recording the authorize address, a wrong code with its error, and a made-up code reaching the prompt. Nothing there signs in to anything.
* **Not checked:** a real login completing, which needs Kai's account and is not run from a seat. Narrow Claude Code screens: the 44 and 34 column captures did not complete, so the wrapped link and menu are shaped by test. The Console and third-party options, whose authorize addresses were not captured. The card on a phone, where `openExternal` hands the link to the Custom Tab. The `/login` command inside a signed-in seat reaches the same menu and is not captured either.
