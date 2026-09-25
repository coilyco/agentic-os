<script lang="ts">
  import type { HostConnection, Session } from "../lib/protocol";

  let { connection, session }: { connection: HostConnection; session: Session } = $props();

  let text = $state("");
  let status = $state("");
  const PASTE_OPEN = "\x1b[200~";
  const PASTE_CLOSE = "\x1b[201~";

  // A program in bracketed-paste mode takes the text as one paste, then the
  // Enter a beat later so it submits instead of landing as a newline.
  function send(): void {
    const body = text.trim();
    if (!body) return;
    if (session.paste) {
      connection.input(session.id, `${PASTE_OPEN}${body}${PASTE_CLOSE}`);
      setTimeout(() => connection.input(session.id, "\r"), 300);
    } else {
      connection.input(session.id, `${body.replace(/\r?\n/g, " ")}\r`);
    }
    text = "";
    status = `Sent to ${session.identity}.`;
  }

  function keydown(event: KeyboardEvent): void {
    if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      send();
    }
  }
</script>

<form class="composer" onsubmit={(event) => { event.preventDefault(); send(); }}>
  <label for="composer-input" class="visually-hidden">Message {session.identity}</label>
  <textarea
    id="composer-input"
    bind:value={text}
    onkeydown={keydown}
    rows="1"
    placeholder={`Type or dictate to ${session.identity}. Enter sends, Shift+Enter for a new line.`}
    autocomplete="off"
    spellcheck="true"
  ></textarea>
  <button class="button primary" type="submit" disabled={!text.trim()}>Send</button>
  <p class="visually-hidden" role="status">{status}</p>
</form>

<style>
  .composer { display: flex; gap: 10px; align-items: flex-end; padding: 10px 12px; border-top: 1px solid var(--line); background: var(--ground); }
  textarea {
    flex: 1; min-height: 44px; max-height: 40vh; field-sizing: content; resize: none;
    padding: 11px 14px; border-radius: 10px; border: 1px solid var(--control-line);
    background: var(--terminal); color: var(--text); font: 15px/1.4 var(--font-body);
  }
  textarea:focus-visible { border-color: var(--brand); }
  button:disabled { opacity: 0.5; cursor: default; }
</style>
