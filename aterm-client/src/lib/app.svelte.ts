import { nextUnseen } from "./activity";
import { DaemonHost, DEFAULT_DAEMON_URL, probe } from "./daemon-host";
import { upsertMessage } from "./messages";
import { MockHost } from "./mock-host";
import type { Ask, Host, HostConnection, LaunchState, PeerMessage, Session } from "./protocol";
import type { Role } from "./roster";

export const app = $state({
  hosts: [
    { id: "local", label: "this Mac", address: DEFAULT_DAEMON_URL, kind: "daemon", status: { kind: "checking" } },
    { id: "demo", label: "Demo host", address: "scripted, no daemon", kind: "demo", status: { kind: "online", sessionCount: 4 } },
  ] as Host[],
  selectedHostId: null as string | null,
  attachedHostId: null as string | null,
  connection: null as HostConnection | null,
  roles: [] as Role[],
  sessions: [] as Session[],
  messages: [] as PeerMessage[],
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
});

export async function checkHost(host: Host): Promise<void> {
  if (host.kind !== "daemon") return;
  host.status = { kind: "checking" };
  try {
    host.status = { kind: "online", sessionCount: await probe(host.address) };
  } catch {
    host.status = { kind: "unreachable", reason: "No daemon answered." };
    return;
  }
  // Picked while the probe was still out, so attach now that it answered.
  if (app.selectedHostId === host.id && app.attachedHostId !== host.id) selectHost(host);
}

export function selectedHost(): Host | undefined {
  return app.hosts.find((host) => host.id === app.selectedHostId);
}

/** Selecting an online host attaches to it, so one tap reaches its seats. */
export function selectHost(host: Host): void {
  app.selectedHostId = host.id;
  app.selectedRole = null;
  app.notice = "";
  if (host.status.kind !== "online" || app.attachedHostId === host.id) return;
  app.connection?.close();
  app.roles = [];
  app.sessions = [];
  app.messages = [];
  app.asks = {};
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
    else if (event.type === "launch") app.launches[event.role] = { state: event.state, text: event.text };
    else if (event.type === "closed") {
      host.status = { kind: "unreachable", reason: event.reason };
      app.attachedHostId = null;
      app.connection = null;
      app.selectedRole = null;
    }
  });
}

export function selectRole(slug: string): void {
  app.selectedRole = slug;
  const session = sessionFor(slug);
  if (session) delete app.unseen[session.id];
}

function viewingSession(): string | null {
  return app.selectedRole ? (sessionFor(app.selectedRole)?.id ?? null) : null;
}

export function sessionFor(slug: string): Session | undefined {
  return app.sessions.find((session) => session.role === slug);
}

export function colorOf(slug: string): string {
  return app.roles.find((role) => role.slug === slug)?.color ?? "#a4abb9";
}

/** Messages to or from one session. A target may name its role, identity, or session. */
export function messagesFor(session: Session): PeerMessage[] {
  const names = [session.role, session.identity, session.id];
  return app.messages.filter((message) => message.from.role === session.role || message.session === session.id || names.includes(message.target));
}

for (const host of app.hosts) void checkHost(host);

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
  selectRole(next.role);
  if (next.kind === "asking") app.focusCard++;
  return true;
}
