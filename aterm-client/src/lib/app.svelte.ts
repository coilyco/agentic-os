import { upsertMessage } from "./messages";
import { MockHost, mockHosts } from "./mock-host";
import type { Host, HostConnection, PeerMessage, Session } from "./protocol";
import type { Role } from "./roster";

export const app = $state({
  hosts: mockHosts as Host[],
  selectedHostId: null as string | null,
  attachedHostId: null as string | null,
  connection: null as HostConnection | null,
  roles: [] as Role[],
  sessions: [] as Session[],
  messages: [] as PeerMessage[],
  selectedRole: null as string | null,
  notice: "",
});

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
  const connection = new MockHost();
  app.connection = connection;
  app.attachedHostId = host.id;
  connection.subscribe((event) => {
    if (event.type === "roster") app.roles = event.roles;
    else if (event.type === "sessions") app.sessions = event.sessions;
    else if (event.type === "message") app.messages = upsertMessage(app.messages, event.message);
  });
}

export function selectRole(slug: string): void {
  app.selectedRole = slug;
}

export function sessionFor(slug: string): Session | undefined {
  return app.sessions.find((session) => session.role === slug);
}

export function colorOf(slug: string): string {
  return app.roles.find((role) => role.slug === slug)?.color ?? "#a4abb9";
}
