import { nextUnseen } from "./activity";
import { type AttentionSettings, loadAttention, storeAttention } from "./attention";
import { chime, isBlocked, unlock, watchBlocked } from "./chime";
import { DaemonHost, DEFAULT_DAEMON_URL, hostLabel, probe } from "./daemon-host";
import { discover, listedAddresses, newFoundHosts, staleFoundHosts } from "./discovery";
import { upsertMessage } from "./messages";
import { MockHost, terminalModeFrom } from "./mock-host";
import type { View } from "./mcp-apps";
import type { SharedBrowser } from "./screencast";
import { loadSavedHosts, parseHostInput, storeSavedHosts } from "./saved-hosts";
import type { Ask, Host, HostConnection, LaunchState, Link, PeerMessage, Session } from "./protocol";
import type { Role } from "./roster";
import { applyList, claim, loadClosed, loadSeats, NO_ANSWER, OPEN_TIMEOUT_MS, recordExit, storeClosed, storeSeats, terminalFor, type Support, type TerminalEntry } from "./terminals";
import { TYPING_OPEN, type Typing } from "./typing";

/** The coilyco.dev build: no daemon serves the page, so hosts are added by hand. */
export const HOSTED = import.meta.env.VITE_ATERM_HOSTED === "1";

function savedHost(saved: { label: string; address: string }): Host {
  return { id: `saved:${saved.address}`, label: saved.label, address: saved.address, kind: "daemon", status: { kind: "checking" } };
}

export const app = $state({
  hosts: [
    ...(HOSTED ? [] : [{ id: "local", label: hostLabel(location, import.meta.env.DEV), address: DEFAULT_DAEMON_URL, kind: "daemon", status: { kind: "checking" } } as Host]),
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
  roles: [] as Role[],
  sessions: [] as Session[],
  messages: [] as PeerMessage[],
  /** The session on screen. Instances are peers, so selection is by session, not role. */
  selectedSession: null as string | null,
  /** A role picked to start, when no session is on screen. */
  selectedRole: null as string | null,
  notice: "",
  launches: {} as Record<string, { state: LaunchState; text: string }>,
  /** Sessions that finished a turn while you were looking at another one. */
  unseen: {} as Record<string, boolean>,
  /** Seats waiting on you, for cues. Unlike `unseen`, counts the open seat if hidden. */
  attention: {} as Record<string, boolean>,
  /** Whether the tab is on screen. The open seat is only "seen" while it is. */
  pageVisible: document.visibilityState === "visible",
  /** The opt-in cues, per browser. `soundBlocked` is the browser refusing audio. */
  alerts: { ...loadAttention(), soundBlocked: false } as AttentionSettings & { soundBlocked: boolean },
  /** ask_choice calls waiting on a person, by ask id. */
  asks: {} as Record<string, Ask>,
  /** True from alt-tabbing in until you leave, while answers walk the waiting seats. */
  triage: false,
  /** Bumped to ask the visible choice card to take keyboard focus. */
  focusCard: 0,
  /** MCP Apps views by view id, newest last. */
  views: {} as Record<string, View>,
  /** Each seat's shared browser, by session id. */
  browsers: {} as Record<string, SharedBrowser>,
  /** Plain shells the host holds, tagged with the seat each sits beside. */
  terminals: [] as TerminalEntry[],
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

/** True while the attached host is silent and the screen is its last known state. */
export function reconnecting(): boolean {
  return app.link.state === "reconnecting";
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
    host.status = { kind: "unreachable", reason: "Nothing answered, so the daemon is stopped or its tailnet listener is failing TLS. A browser cannot tell which." };
    return;
  }
  // Picked while the probe was still out, so attach now that it answered.
  if (app.selectedHostId === host.id && app.attachedHostId !== host.id) selectHost(host);
  if (!host.found) void refreshFound();
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
export function selectHost(host: Host): void {
  app.selectedHostId = host.id;
  app.selectedSession = null;
  app.selectedRole = null;
  app.notice = "";
  if (host.status.kind !== "online" || app.attachedHostId === host.id) return;
  app.connection?.close();
  clearTimeout(restoredTimer);
  app.link = { state: "live" };
  app.newerBuild = false;
  app.typing = TYPING_OPEN;
  app.roles = [];
  app.sessions = [];
  app.messages = [];
  app.asks = {};
  app.attention = {};
  app.views = {};
  app.browsers = {};
  app.terminals = [];
  app.terminalSupport = "unknown";
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
      app.attention = nextUnseen(app.sessions, event.sessions, openSeat(), app.attention);
      app.sessions = event.sessions;
    }
    else if (event.type === "message") app.messages = upsertMessage(app.messages, event.message);
    else if (event.type === "typing") app.typing = event.typing;
    else if (event.type === "notice") app.notice = event.text;
    else if (event.type === "ask") app.asks[event.ask.id] = event.ask;
    else if (event.type === "asked") delete app.asks[event.id];
    else if (event.type === "view") app.views[event.view.id] = event.view;
    else if (event.type === "view_update") {
      const view = app.views[event.id];
      if (view) Object.assign(view, { toolResult: event.toolResult ?? view.toolResult, cancelled: event.cancelled ?? view.cancelled });
    }
    else if (event.type === "view_closed") delete app.views[event.id];
    else if (event.type === "browser") app.browsers[event.browser.session] = event.browser;
    else if (event.type === "features") app.terminalSupport = event.terminals ? "yes" : "no";
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
    else if (event.type === "launch") app.launches[event.role] = { state: event.state, text: event.text };
    else if (event.type === "reconnecting") app.link = { state: "reconnecting", attempt: event.attempt, retryAt: event.retryAt };
    else if (event.type === "reconnected") {
      // The new connection re-sends pending asks. The old ones belong to a dead daemon.
      app.asks = {};
      app.link = { state: "restored" };
      clearTimeout(restoredTimer);
      restoredTimer = setTimeout(() => {
        if (app.link.state === "restored") app.link = { state: "live" };
      }, RESTORED_MS);
      // A hosted page is not served by the daemon, so a reload brings no new build.
      if (event.newerBuild && !HOSTED) app.newerBuild = true;
    }
    else if (event.type === "closed") {
      clearTimeout(restoredTimer);
      app.link = { state: "live" };
      host.status = { kind: "unreachable", reason: event.reason };
      app.attachedHostId = null;
      app.connection = null;
      app.selectedSession = null;
      app.selectedRole = null;
    }
  });
}

export function selectSession(id: string): void {
  app.selectedSession = id;
  app.selectedRole = null;
  delete app.unseen[id];
  delete app.attention[id];
}

/** Opens a role's start panel. */
export function selectRole(slug: string): void {
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

for (const host of app.hosts) void checkHost(host);

/** A session's views, newest first. */
export function viewsFor(sessionId: string): View[] {
  return Object.values(app.views).filter((view) => view.session === sessionId).reverse();
}

export function asksFor(sessionId: string): Ask[] {
  return Object.values(app.asks).filter((ask) => ask.session === sessionId);
}

export interface Waiting {
  sessionId: string;
  role: string;
  identity: string;
  kind: "asking" | "done";
}

/** Seats that need you, asks before finished turns, in the order they arrived. */
export function waitingSeats(): Waiting[] {
  const bySession = new Map(app.sessions.map((session) => [session.id, session]));
  const asking = Object.values(app.asks)
    .map((ask) => bySession.get(ask.session))
    .filter((session): session is Session => session !== undefined);
  const done = Object.keys(app.unseen)
    .map((id) => bySession.get(id))
    .filter((session): session is Session => session !== undefined && !asking.includes(session));
  const seen = new Set<string>();
  return [
    ...asking.map((session) => ({ session, kind: "asking" as const })),
    ...done.map((session) => ({ session, kind: "done" as const })),
  ]
    .filter(({ session }) => (seen.has(session.id) ? false : (seen.add(session.id), true)))
    .map(({ session, kind }) => ({ sessionId: session.id, role: session.role, identity: session.identity, kind }));
}

/** Opens the seat that most needs you and hands its answer card the keyboard. */
export function jumpToWaiting(): boolean {
  const next = waitingSeats()[0];
  if (!next) return false;
  selectSession(next.sessionId);
  if (next.kind === "asking") app.focusCard++;
  return true;
}

/** The seat on screen, while the tab is: the one seat the cues leave alone. */
export function openSeat(): string | null {
  return app.pageVisible ? app.selectedSession : null;
}

/** Who is waiting, for cues. A read-only browser cannot answer, so its asks sit out. */
export function cueSeats(): string[] {
  const live = new Set(app.sessions.map((session) => session.id));
  const asking = app.typing.allowed ? Object.values(app.asks).map((ask) => ask.session) : [];
  return [...new Set([...Object.keys(app.attention), ...asking])].filter((id) => live.has(id));
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
