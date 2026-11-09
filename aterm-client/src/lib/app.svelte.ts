import { nextUnseen } from "./activity";
import { EXTERNAL, PLACE, type Place } from "./build";
import { bridge } from "./device-key";
import { type AttentionSettings, loadAttention, storeAttention } from "./attention";
import { chime, isBlocked, unlock, watchBlocked } from "./chime";
import { DaemonHost, DEFAULT_DAEMON_URL, hostLabel, probe } from "./daemon-host";
import { discover, listedAddresses, newFoundHosts, staleFoundHosts } from "./discovery";
import { upsertMessage } from "./messages";
import { askerLine, cardSeatOf } from "./relay";
import { MockHost, terminalModeFrom } from "./mock-host";
import { diagnose } from "./reach";
import { Recheck } from "./recheck";
import type { View } from "./mcp-apps";
import type { SharedBrowser } from "./screencast";
import { loadSavedHosts, parseHostInput, storeSavedHosts } from "./saved-hosts";
import type { Ask, Host, HostConnection, LaunchState, Link, PeerMessage, SeatArtifact, Session } from "./protocol";
import { seatsNeedingKai, type NeedsKai } from "./needs-kai";
import { closedWords, fullyIdle } from "./idle-close";
import type { ResumeState } from "./resume";
import type { Role } from "./roster";
import { applyList, claim, loadClosed, loadSeats, NO_ANSWER, OPEN_TIMEOUT_MS, recordExit, storeClosed, storeSeats, terminalFor, type Support, type TerminalEntry } from "./terminals";
import { TYPING_OPEN, type Typing } from "./typing";
import { offer, pageLabel, type OpenedPage } from "./pages";
import { loadLastOpen, seatToRestore, storeLastOpen, type LastOpen } from "./last-open";
import { beginLaunch, newSeatFor, type PendingLaunch } from "./launch-follow";

/** The coilyco.dev build: no daemon serves the page, so hosts are added by hand. */

function savedHost(saved: { label: string; address: string }): Host {
  return { id: `saved:${saved.address}`, label: saved.label, address: saved.address, kind: "daemon", status: { kind: "checking" } };
}

/** A phone on its side: too short to read the desktop layout in.
 *  A short desktop window has a fine pointer, so it keeps its layout. */
const READING = "(orientation: landscape) and (max-height: 500px) and (pointer: coarse)";
export const layout = $state({ reading: typeof matchMedia === "function" && matchMedia(READING).matches });
if (typeof matchMedia === "function") matchMedia(READING).addEventListener("change", (event) => (layout.reading = event.matches));

export const app = $state({
  hosts: [
    ...(EXTERNAL ? [] : [{ id: "local", label: hostLabel(location, import.meta.env.DEV), address: DEFAULT_DAEMON_URL, kind: "daemon", status: { kind: "checking" } } as Host]),
    ...loadSavedHosts().map(savedHost),
    { id: "demo", label: "Demo host", address: "scripted, no daemon", kind: "demo", status: { kind: "online", sessionCount: 4 } },
  ] as Host[],
  selectedHostId: null as string | null,
  attachedHostId: null as string | null,
  connection: null as HostConnection | null,
  /** Whether the attached host answers. If not, the screen is its last known state. */
  link: { state: "live" } as Link,
  /** A returning daemon is a newer build than this page, so a reload brings it. */
  newerBuild: false,
  /** Whether the daemon's guard lets this connection type. A mock host never refuses. */
  typing: TYPING_OPEN as Typing,
  /** The host lists device-key and this page has the app's plugin. */
  deviceKeyAvailable: false,
  /** A held unlock proof is out, or the daemon said it ended. Null otherwise. */
  resume: null as ResumeState | null,
  /** The welcome lists passkey. Reactive, which the connection's feature list is not. */
  passkeyAvailable: false,
  roles: [] as Role[],
  sessions: [] as Session[],
  messages: [] as PeerMessage[],
  /** The session on screen. Instances are peers, so selection is by session, not role. */
  selectedSession: null as string | null,
  /** A role picked to start, when no session is on screen. */
  selectedRole: null as string | null,
  notice: "",
  /** Pages seats asked to open. A card offers each, and nothing moves until a click. */
  openedPages: [] as OpenedPage[],
  /** The page a click is loading into a seat's pane, until it holds control and loads. */
  pageToShow: null as { sessionId: string; url: string; since: number } | null,
  launches: {} as Record<string, { state: LaunchState; text: string }>,
  /** Sessions that finished a turn while you were looking at another one. */
  unseen: {} as Record<string, boolean>,
  /** Whether the tab is on screen. The open seat is only "seen" while it is. */
  pageVisible: document.visibilityState === "visible",
  /** The opt-in cues, per browser. `soundBlocked` is the browser refusing audio. */
  alerts: { ...loadAttention(), soundBlocked: false } as AttentionSettings & { soundBlocked: boolean },
  /** ask_choice calls waiting on a person, by ask id. */
  asks: {} as Record<string, Ask>,
  /** The control asking "Close it?" (row:id or view:id), and the last result. */
  closing: null as string | null,
  closeNote: "",
  /** What the note offers: close it anyway, or reopen what was closed. */
  closeAction: null as { kind: "force" | "reopen"; session: string; ref: string } | null,
  /** Pages seats put on her desk, by id. One marked waiting is a seat that needs her. */
  artifacts: {} as Record<string, SeatArtifact>,
  /** Bumped when a person opens a seat with an ask, so its card takes the keyboard. */
  focusCard: 0,
  /** The seat whose card was asked for the keyboard. Spent once it has it. */
  focusAsk: null as string | null,
  /** The composer has focus. On a phone that is the soft keyboard being up. */
  composing: false,
  /** What was typed and not sent, by seat, so a lock or a seat change never takes it. */
  drafts: {} as Record<string, string>,
  /** Why typed input was not taken. Stays until dismissed or the next send. */
  inputNotice: "",
  /** MCP Apps views by view id, newest last. */
  views: {} as Record<string, View>,
  /** Each seat's shared browser, by session id. */
  browsers: {} as Record<string, SharedBrowser>,
  /** Plain shells the host holds, tagged with the seat each sits beside. */
  terminals: [] as TerminalEntry[],
  /** Whether the host's welcome lists `web-push`, so the Web Push switch shows. */
  webPush: false,
  /** Whether the host's welcome lists the terminal feature. Unknown until it arrives. */
  terminalSupport: "unknown" as Support,
  /** Terminal id to the seat it was opened beside. The host does not say. */
  terminalSeats: loadSeats(),
  /** The host's first terminal list has arrived, so an empty one means none. */
  terminalsLoaded: false,
  /** Seats whose terminal is being opened, and the host's reason when it would not. */
  terminalOpening: {} as Record<string, true>,
  terminalRefusals: {} as Record<string, string>,
  /** Seats Kai closed a terminal for, so opening their tab does not bring it back. */
  terminalClosed: loadClosed(),
});

// Ids Kai closed: they leave the list on purpose, so they are not an ended shell.
const dismissed = new Set<string>();
// "Back" is said once and then leaves, so the line does not outlive the news.
const RESTORED_MS = 5000;
let restoredTimer: ReturnType<typeof setTimeout> | undefined;

/** True while the attached host is silent and the screen is its last known state. That
 * holds in the hour of tries and after it, when only a return to the page dials again. */
export function reconnecting(): boolean {
  return app.link.state === "reconnecting" || app.link.state === "resting";
}

/** Brings the attached host's own panel forward, where its connection check is. */
export function showHost(): void {
  restoring = null;
  launching = null;
  if (app.attachedHostId) storeLastOpen({ host: app.attachedHostId });
  app.selectedSession = null;
  app.selectedRole = null;
  if (app.attachedHostId) app.selectedHostId = app.attachedHostId;
}

/** Asks the connection to dial now. A host that does not redial has nothing to hurry. */
export function retryNow(): void {
  app.connection?.retry?.();
}
const openTimers = new Map<string, ReturnType<typeof setTimeout>>();

function settleOpen(label: string): void {
  clearTimeout(openTimers.get(label));
  openTimers.delete(label);
  delete app.terminalOpening[label];
}

/** A shell that arrived after a timed-out spawn makes that refusal wrong. */
function shellArrived(label: string): void {
  settleOpen(label);
  delete app.terminalRefusals[label];
}

/** Opens a shell beside a seat. Asking again is what clears a refusal or an exit. */
export function openTerminal(sessionId: string): void {
  const channel = app.connection?.terminals;
  if (!channel || app.terminalSupport !== "yes" || app.terminalOpening[sessionId]) return;
  delete app.terminalRefusals[sessionId];
  app.terminalClosed = app.terminalClosed.filter((each) => each !== sessionId);
  storeClosed(app.terminalClosed);
  app.terminals = app.terminals.filter((entry) => !(entry.label === sessionId && entry.exit));
  app.terminalOpening[sessionId] = true;
  openTimers.set(
    sessionId,
    setTimeout(() => {
      settleOpen(sessionId);
      app.terminalRefusals[sessionId] = NO_ANSWER;
    }, OPEN_TIMEOUT_MS),
  );
  channel.open(sessionId);
}

/** Ends a seat's shell, or dismisses one that already ended, and keeps it closed. */
export function closeTerminal(sessionId: string): void {
  const entry = terminalFor(app.terminals, sessionId);
  if (!entry) return;
  dismissed.add(entry.id);
  if (entry.exit) app.terminals = app.terminals.filter((each) => each.id !== entry.id);
  else app.connection?.terminals?.close(entry.id);
  app.terminalClosed = [...app.terminalClosed.filter((each) => each !== sessionId), sessionId];
  storeClosed(app.terminalClosed);
}

export async function checkHost(host: Host): Promise<void> {
  if (host.kind !== "daemon") return;
  host.status = { kind: "checking" };
  try {
    host.status = { kind: "online", sessionCount: await probe(host.address) };
    host.answered = true;
  } catch {
    // The socket failure says nothing, so ask over HTTPS which layer stopped it.
    const { layer, message } = await diagnose(host.address);
    host.status = { kind: "unreachable", reason: message, layer };
    watchHost(host);
    return;
  }
  stopWatching(host);
  // Picked while the probe was still out, so attach now that it answered.
  if (app.selectedHostId === host.id && app.attachedHostId !== host.id) selectHost(host);
  if (!host.found) void refreshFound();
}

// Hosts that are not answering are asked again on the backoff and on every return to
// the page, so they never wait for a tap (COI-3352). Keyed by address: ids change.
const rechecks = new Map<string, Recheck>();

function watchHost(host: Host): void {
  if (host.kind !== "daemon") return;
  let recheck = rechecks.get(host.address);
  if (!recheck) {
    recheck = new Recheck(() => recheckHost(host));
    rechecks.set(host.address, recheck);
  }
  recheck.start();
}

function stopWatching(host: Host): void {
  rechecks.get(host.address)?.stop();
  rechecks.delete(host.address);
}

/** The checkHost probe without its Checking screen, so the page does not flicker.
 * Attaches the host if it was the one picked. */
async function recheckHost(host: Host): Promise<boolean> {
  try {
    host.status = { kind: "online", sessionCount: await probe(host.address) };
  } catch {
    return false;
  }
  host.answered = true;
  stopWatching(host);
  if (app.selectedHostId === host.id && app.attachedHostId !== host.id) selectHost(host);
  if (!host.found) void refreshFound();
  return true;
}

if (typeof window !== "undefined" && typeof document !== "undefined") {
  const wake = () => {
    if (document.visibilityState === "visible") for (const recheck of rechecks.values()) recheck.wake();
  };
  window.addEventListener("online", wake);
  window.addEventListener("focus", wake);
  document.addEventListener("visibilitychange", wake);
}

const FOUND_REFRESH_MS = 60_000;
let refreshingFound = false;
let refreshFoundAgain = false;

/** Re-asks each online daemon host, lists new ones, prunes. Why: discovery.md. */
async function refreshFound(): Promise<void> {
  if (refreshingFound) {
    refreshFoundAgain = true;
    return;
  }
  refreshingFound = true;
  try {
    do {
      refreshFoundAgain = false;
      const asked = app.hosts.filter((host) => host.kind === "daemon" && !host.found && host.status.kind === "online");
      const replies = await Promise.allSettled(asked.map((host) => discover(host.address)));
      const found = replies.flatMap((reply) => (reply.status === "fulfilled" ? reply.value : []));
      if (replies.some((reply) => reply.status === "fulfilled")) {
        for (const id of staleFoundHosts(app.hosts, listedAddresses(found))) removeHost(id);
      }
      const added = newFoundHosts(app.hosts, found);
      // Before the demo host, which stays last.
      app.hosts.splice(app.hosts.length - 1, 0, ...added);
      for (const host of added) void checkHost(app.hosts.find((candidate) => candidate.id === host.id)!);
    } while (refreshFoundAgain);
  } finally {
    refreshingFound = false;
  }
}

setInterval(() => {
  if (document.visibilityState === "visible") void refreshFound();
}, FOUND_REFRESH_MS);

/** Keeps a found host on this device, as if it had been typed. */
export function saveFoundHost(host: Host): void {
  if (!host.found) return;
  const id = `saved:${host.address}`;
  if (app.selectedHostId === host.id) app.selectedHostId = id;
  if (app.attachedHostId === host.id) app.attachedHostId = id;
  host.id = id;
  host.found = false;
  persist();
}

/** Adds a host by tailnet name, remembered on this device. Throws a readable message. */
export function addHost(input: string): Host {
  const saved = parseHostInput(input);
  const existing = app.hosts.find((host) => host.address === saved.address);
  if (existing) {
    saveFoundHost(existing);
    return existing;
  }
  const host = savedHost(saved);
  app.hosts.splice(app.hosts.length - 1, 0, host);
  persist();
  void checkHost(app.hosts.find((candidate) => candidate.id === host.id)!);
  return host;
}

export function removeHost(id: string): void {
  if (app.attachedHostId === id) {
    app.connection?.close();
    app.connection = null;
    app.attachedHostId = null;
  }
  const removed = app.hosts.find((host) => host.id === id);
  if (removed) stopWatching(removed);
  app.hosts = app.hosts.filter((host) => host.id !== id);
  if (app.selectedHostId === id) app.selectedHostId = null;
  persist();
}

function persist(): void {
  storeSavedHosts(app.hosts.filter((host) => host.id.startsWith("saved:")).map(({ label, address }) => ({ label, address })));
}

export function selectedHost(): Host | undefined {
  return app.hosts.find((host) => host.id === app.selectedHostId);
}

/** Selecting an online host attaches to it, so one tap reaches its seats. */
// What this device last had open, until the restore uses it or a tap overtakes it.
let restoring: LastOpen | null = loadLastOpen();
// A launch waiting for its seat to be listed. Any other pick by a person ends the wait.
let launching: PendingLaunch | null = null;
let restoringNow = false;

function restoreFor(host: Host): void {
  if (restoring?.host !== host.id || app.selectedHostId || host.status.kind !== "online") return;
  restoringNow = true;
  try {
    selectHost(host);
  } finally {
    restoringNow = false;
  }
}

function restoreSeat(last: LastOpen, listed: readonly Session[]): void {
  const step = seatToRestore(last, listed);
  if (!step) return;
  restoringNow = true;
  try {
    if ("open" in step) selectSession(step.open);
    else app.notice = `${step.gone} is not running any more. Pick a seat from the list.`;
  } finally {
    restoringNow = false;
  }
}

export function selectHost(host: Host): void {
  if (!restoringNow) restoring = null;
  launching = null;
  storeLastOpen({ host: host.id });
  app.selectedHostId = host.id;
  app.selectedSession = null;
  app.selectedRole = null;
  app.notice = "";
  app.openedPages = [];
  app.pageToShow = null;
  if (host.status.kind !== "online" || app.attachedHostId === host.id) return;
  app.connection?.close();
  clearTimeout(restoredTimer);
  app.link = { state: "live" };
  app.newerBuild = false;
  app.typing = TYPING_OPEN;
  app.passkeyAvailable = false;
  app.deviceKeyAvailable = false;
  app.resume = null;
  app.inputNotice = "";
  app.drafts = {};
  app.roles = [];
  app.sessions = [];
  app.messages = [];
  app.asks = {};
  app.artifacts = {};
  app.views = {};
  app.browsers = {};
  app.terminals = [];
  app.terminalSupport = "unknown";
  app.webPush = false;
  app.terminalsLoaded = false;
  for (const label of Object.keys(app.terminalOpening)) settleOpen(label);
  app.terminalRefusals = {};
  const connection = host.kind === "daemon" ? new DaemonHost(host.address) : new MockHost(undefined, terminalModeFrom(typeof location === "undefined" ? "" : location.search));
  app.connection = connection;
  app.attachedHostId = host.id;
  connection.subscribe((event) => {
    if (event.type === "roster") app.roles = event.roles;
    else if (event.type === "sessions") {
      app.unseen = nextUnseen(app.sessions, event.sessions, viewingSession(), app.unseen);
      app.sessions = event.sessions;
      if (restoring?.host === host.id) {
        const last = restoring;
        restoring = null;
        restoreSeat(last, event.sessions);
      }
      const arrived = launching ? newSeatFor(launching, event.sessions, Date.now()) : null;
      if (arrived) selectSession(arrived);
    }
    else if (event.type === "message") app.messages = upsertMessage(app.messages, event.message);
    else if (event.type === "typing") {
      app.typing = event.typing;
      if (event.typing.allowed) app.resume = null;
    }
    else if (event.type === "resume") app.resume = event.state;
    else if (event.type === "notice") app.notice = event.text;
    else if (event.type === "open_url") offerPage(event.session, event.url);
    else if (event.type === "input_refused") app.inputNotice = event.text;
    else if (event.type === "ask") app.asks[event.ask.id] = event.ask;
    else if (event.type === "asked") delete app.asks[event.id];
    else if (event.type === "artifact") app.artifacts[event.artifact.id] = event.artifact;
    else if (event.type === "artifact_update") {
      const page = app.artifacts[event.id];
      if (page) Object.assign(page, { waiting: event.waiting ?? page.waiting, seatEnded: event.seatEnded ?? page.seatEnded });
    }
    else if (event.type === "artifact_closed") delete app.artifacts[event.id];
    else if (event.type === "view") app.views[event.view.id] = event.view;
    else if (event.type === "view_update") {
      const view = app.views[event.id];
      if (view) Object.assign(view, { toolResult: event.toolResult ?? view.toolResult, cancelled: event.cancelled ?? view.cancelled });
    }
    else if (event.type === "view_closed") delete app.views[event.id];
    else if (event.type === "browser") app.browsers[event.browser.session] = event.browser;
    else if (event.type === "passkey") app.passkeyAvailable = event.available;
    else if (event.type === "device_key") app.deviceKeyAvailable = event.available && bridge() !== null;
    else if (event.type === "features") {
      app.terminalSupport = event.terminals ? "yes" : "no";
      app.webPush = event.webPush === true;
    }
    else if (event.type === "terminals") {
      app.terminals = applyList(app.terminals, event.terminals, app.terminalSeats, dismissed);
      app.terminalsLoaded = true;
      for (const each of app.terminals) if (each.label && !each.exit) shellArrived(each.label);
    }
    else if (event.type === "terminal_opened") {
      app.terminalSeats[event.id] = event.label;
      storeSeats(app.terminalSeats);
      app.terminals = claim(app.terminals, event.id, event.label);
      // The reply can beat the list, so opening ends when the list shows the shell.
      if (app.terminals.some((entry) => entry.id === event.id)) shellArrived(event.label);
    }
    else if (event.type === "terminal_exit") app.terminals = recordExit(app.terminals, event.id, event.code);
    else if (event.type === "terminal_refused") {
      settleOpen(event.label);
      app.terminalRefusals[event.label] = event.text;
    }
    else if (event.type === "seat_closed") {
      const gone = closePending;
      closePending = null;
      if (gone?.id === event.session) {
        const canReopen = app.connection?.canReopen === true;
        app.closeNote = closedWords(gone.identity, gone.roleName, canReopen);
        app.closeAction = canReopen ? { kind: "reopen", session: gone.id, ref: gone.ref } : null;
        if (app.selectedSession === gone.id) showHost();
      }
    }
    else if (event.type === "seat_close_refused") {
      app.closeNote = event.text;
      // Work on no remote is the person's call: close again, forced, after a second ask.
      app.closeAction = event.unpushed && closePending ? { kind: "force", session: closePending.id, ref: closePending.ref } : null;
      if (!event.unpushed) closePending = null;
    }
    else if (event.type === "seat_reopened") {
      app.closeNote = "Reopened. It opens here once the host lists it.";
      app.closeAction = null;
      // A fresh name, so it is found as a launch is: the first new seat of its role.
      launching = beginLaunch(event.role, app.sessions, Date.now());
    }
    else if (event.type === "seat_reopen_refused") {
      app.closeNote = event.text;
      app.closeAction = null;
    }
    else if (event.type === "launch") {
      app.launches[event.role] = { state: event.state, text: event.text };
      // Watch from the tap: the seat can be listed before `launched`, or after it.
      if (event.state === "failed") launching = null;
      else if (event.state === "starting" || launching?.role !== event.role) launching = beginLaunch(event.role, app.sessions, Date.now());
    }
    else if (event.type === "reconnecting") app.link = { state: "reconnecting", attempt: event.attempt, retryAt: event.retryAt };
    else if (event.type === "resting") app.link = { state: "resting" };
    else if (event.type === "reconnected") {
      app.resume = null;
      // The new connection re-sends pending asks. The old ones belong to a dead daemon.
      app.asks = {};
      app.artifacts = {};
      app.link = { state: "restored" };
      clearTimeout(restoredTimer);
      restoredTimer = setTimeout(() => {
        if (app.link.state === "restored") app.link = { state: "live" };
      }, RESTORED_MS);
      // A hosted page is not served by the daemon, so a reload brings no new build.
      if (event.newerBuild && !EXTERNAL) app.newerBuild = true;
    }
    else if (event.type === "closed") {
      clearTimeout(restoredTimer);
      launching = null;
      app.link = { state: "live" };
      host.status = { kind: "unreachable", reason: event.reason };
      watchHost(host);
      app.attachedHostId = null;
      app.connection = null;
      app.selectedSession = null;
      app.selectedRole = null;
    }
  });
}

export function selectSession(id: string): void {
  if (!restoringNow) restoring = null;
  app.closing = null;
  launching = null;
  const seat = sessionById(id);
  if (seat && app.attachedHostId) storeLastOpen({ host: app.attachedHostId, seat: { id, who: seat.identity } });
  app.selectedSession = id;
  app.selectedRole = null;
  delete app.unseen[id];
}

let closePending: { id: string; ref: string; identity: string; role: string; roleName: string } | null = null;

/** Seats a card or a waiting page concerns, as one set so a row costs a lookup. */
export function needsKaiIds(): Set<string> {
  return new Set([...waitingSeats().map((seat) => seat.sessionId), ...Object.values(app.asks).map((ask) => ask.session)]);
}

/** Whether to offer Close. A read-only or redialing connection offers none. */
export function closable(session: Session, needsKai: boolean): boolean {
  if (!app.connection?.closeSeat || !app.typing.allowed || reconnecting()) return false;
  return fullyIdle(session, { needsKai, unseen: !!app.unseen[session.id] });
}

/** Closes the seat the person confirmed, by its ref and never its name. */
export function closeSeatNow(session: Session): void {
  app.closing = null;
  if (!session.ref || !app.connection?.closeSeat) return;
  closePending = { id: session.id, ref: session.ref, identity: session.identity, role: session.role, roleName: app.roles.find((role) => role.slug === session.role)?.displayName ?? session.role };
  app.closeNote = "";
  app.closeAction = null;
  app.connection.closeSeat(session.id, session.ref);
}

/** The close that was refused for unpushed work, sent again as the person's decision. */
export function closeAnyway(): void {
  const again = app.closeAction;
  app.closeAction = null;
  if (again?.kind === "force" && closePending?.id === again.session) app.connection?.closeSeat?.(again.session, again.ref, true);
}

/** Brings back the seat just closed, under the fresh name the daemon gives it. */
export function reopenClosed(): void {
  const again = app.closeAction;
  app.closeAction = null;
  if (again?.kind === "reopen") app.connection?.reopenSeat?.(again.session, again.ref);
}

/** Puts the note away: the seat stays open, or the closed one stays closed. */
export function dismissCloseNote(): void {
  app.closeNote = "";
  app.closeAction = null;
  closePending = null;
}

/** Opens a role's start panel. */
export function selectRole(slug: string): void {
  restoring = null;
  launching = null;
  app.selectedRole = slug;
  app.selectedSession = null;
}

function viewingSession(): string | null {
  return app.selectedSession;
}

export function sessionById(id: string | null): Session | undefined {
  return id ? app.sessions.find((session) => session.id === id) : undefined;
}

/** A role's most recent failed launch, the one thing its start panel still reports. */
export function failedLaunchOf(slug: string): Session | undefined {
  return app.sessions.find((session) => session.role === slug && session.state === "failed");
}

export function colorOf(slug: string): string {
  return app.roles.find((role) => role.slug === slug)?.color ?? "#a4abb9";
}

/** Messages to or from one session. A target may name its role, identity, or session. */
export function messagesFor(session: Session): PeerMessage[] {
  const names = [session.role, session.identity, session.id];
  // A delivered message names its session, so a sibling instance does not claim it.
  const inbound = (message: PeerMessage) => (message.session ? message.session === session.id : names.includes(message.target));
  return app.messages.filter((message) => message.from.role === session.role || inbound(message));
}

for (const host of app.hosts) void checkHost(host).then(() => restoreFor(host));

/** A session's views, newest first. */
export function viewsFor(sessionId: string): View[] {
  return Object.values(app.views).filter((view) => view.session === sessionId).reverse();
}

export function asksFor(sessionId: string): Ask[] {
  return Object.values(app.asks).filter((ask) => ask.session === sessionId);
}

// One Set per roster: a row building its own made a frame cost N squared (COI-3363).
// Keyed by the array a frame replaces. A $derived recomputes when nothing watches it.
let liveCache: { list: unknown; size: number; ids: Set<string> } | null = null;
function liveIds(): Set<string> {
  const list = app.sessions;
  if (liveCache?.list !== list || liveCache.size !== list.length) liveCache = { list, size: list.length, ids: new Set(list.map((session) => session.id)) };
  return liveCache.ids;
}

/** Asks whose card shows on this seat's pane: its own, and a worker's relayed to it. */
export function asksShownOn(sessionId: string): Ask[] {
  const live = liveIds();
  return Object.values(app.asks).filter((ask) => cardSeatOf(ask, live) === sessionId);
}

/** The seat showing the card for a question this seat asked, if another seat. */
export function relayedTo(sessionId: string): Session | undefined {
  const live = liveIds();
  for (const ask of Object.values(app.asks)) {
    if (ask.session !== sessionId) continue;
    const seat = cardSeatOf(ask, live);
    if (seat !== sessionId) return app.sessions.find((session) => session.id === seat);
  }
  return undefined;
}

/** What a row says about an ask that concerns its seat, or null when none does. */
export function askLineFor(sessionId: string): string | null {
  // Almost always no ask is open, so a row costs nothing and reads no seat.
  if (Object.keys(app.asks).length === 0) return null;
  const live = liveIds();
  const nameOf = (id: string) => app.sessions.find((session) => session.id === id)?.identity ?? id;
  const ask = Object.values(app.asks).find((each) => each.session === sessionId || cardSeatOf(each, live) === sessionId);
  return ask ? askerLine(ask, sessionId, live, nameOf) : null;
}

export type Waiting = NeedsKai;

/** Seats that need you: the one list the title, badge, chip, toast and cues all read. */
export function waitingSeats(): Waiting[] {
  return seatsNeedingKai(Object.values(app.asks), Object.values(app.artifacts), app.sessions);
}

let nextPage = 0;

/** A seat asked to open a page. It becomes a card, and the screen stays where it is. */
export function offerPage(sessionId: string, url: string): void {
  app.openedPages = offer(app.openedPages, { sessionId, url, at: Date.now() }, ++nextPage);
}

export function dismissPage(id: number): void {
  app.openedPages = app.openedPages.filter((page) => page.id !== id);
}

/** The click on a card: that seat on screen, its browser showing, the page loading. */
export function showPage(id: number): void {
  const page = app.openedPages.find((each) => each.id === id);
  if (!page) return;
  dismissPage(id);
  const seat = sessionById(page.sessionId);
  if (!seat) {
    app.notice = `That seat has ended, so ${pageLabel(page.url)} was not opened.`;
    return;
  }
  if (!app.connection?.browser) {
    app.notice = `This host does not stream a browser, so ${pageLabel(page.url)} was not opened here.`;
    return;
  }
  openByPerson(seat.id);
  app.pageToShow = { sessionId: seat.id, url: page.url, since: Date.now() };
}

// A person opened this seat by click or tap, the only way one is selected.
// A pending ask takes the keyboard, as it does for anyone who opens its seat.
export function openByPerson(id: string): void {
  selectSession(id);
  if (asksShownOn(id).length) {
    app.focusAsk = id;
    app.focusCard++;
  }
}

/** The seat on screen, while the tab is: the one seat the cues leave alone. */
export function openSeat(): string | null {
  return app.pageVisible ? app.selectedSession : null;
}

/** Who is waiting, for cues. A read-only browser cannot answer, so its asks sit out. */
export function cueSeats(): string[] {
  const asks = app.typing.allowed ? Object.values(app.asks) : [];
  return seatsNeedingKai(asks, Object.values(app.artifacts), app.sessions).map((seat) => seat.sessionId);
}

/** Waiting seats the strong visual draws, with their role colour. */
export function glowSeats(): { sessionId: string; color: string }[] {
  const open = openSeat();
  return cueSeats()
    .filter((id) => id !== open)
    .map((sessionId) => ({ sessionId, color: colorOf(app.sessions.find((session) => session.id === sessionId)?.role ?? "") }));
}

watchBlocked((blocked) => (app.alerts.soundBlocked = app.alerts.sound && blocked));
// A reload with Sound on starts with audio shut until the first touch of the page.
if (app.alerts.sound) app.alerts.soundBlocked = isBlocked();

/** Flips a cue. Turning Sound on wakes audio and plays the cue once to prove it. */
export async function setAlert(key: keyof AttentionSettings, on: boolean): Promise<void> {
  app.alerts[key] = on;
  storeAttention({ sound: app.alerts.sound, visual: app.alerts.visual });
  if (key !== "sound") return;
  if (!on) return void (app.alerts.soundBlocked = false);
  if (await unlock()) chime();
}

/** Plays the cue if Sound is on. */
export function playCue(): void {
  if (app.alerts.sound) chime();
}

/** Where this page unlocks from, for the words a locked device shows. */
export function placeNow(): Place {
  return app.deviceKeyAvailable ? "app-key" : PLACE;
}
