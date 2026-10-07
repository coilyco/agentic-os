import { nextUnseen } from "./activity";
import { DaemonHost, DEFAULT_DAEMON_URL, hostLabel, probe } from "./daemon-host";
import { upsertMessage } from "./messages";
import { MockHost } from "./mock-host";
import type { View } from "./mcp-apps";
import type { SharedBrowser } from "./screencast";
import { loadSavedHosts, parseHostInput, storeSavedHosts } from "./saved-hosts";
import type { Ask, Host, HostConnection, LaunchState, PeerMessage, Session } from "./protocol";
import type { Role } from "./roster";

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
});

export async function checkHost(host: Host): Promise<void> {
  if (host.kind !== "daemon") return;
  host.status = { kind: "checking" };
  try {
    host.status = { kind: "online", sessionCount: await probe(host.address) };
  } catch {
    host.status = { kind: "unreachable", reason: "Nothing answered, so the daemon is stopped or its tailnet listener is failing TLS. A browser cannot tell which." };
    return;
  }
  // Picked while the probe was still out, so attach now that it answered.
  if (app.selectedHostId === host.id && app.attachedHostId !== host.id) selectHost(host);
}

/** Adds a host by tailnet name, remembered on this device. Throws a readable message. */
export function addHost(input: string): Host {
  const saved = parseHostInput(input);
  const existing = app.hosts.find((host) => host.address === saved.address);
  if (existing) return existing;
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
  app.roles = [];
  app.sessions = [];
  app.messages = [];
  app.asks = {};
  app.views = {};
  app.browsers = {};
  const connection = host.kind === "daemon" ? new DaemonHost(host.address) : new MockHost();
  app.connection = connection;
  app.attachedHostId = host.id;
  connection.subscribe((event) => {
    if (event.type === "roster") app.roles = event.roles;
    else if (event.type === "sessions") {
      app.unseen = nextUnseen(app.sessions, event.sessions, viewingSession(), app.unseen);
      app.sessions = event.sessions;
    }
    else if (event.type === "message") app.messages = upsertMessage(app.messages, event.message);
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
    else if (event.type === "launch") app.launches[event.role] = { state: event.state, text: event.text };
    else if (event.type === "closed") {
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
