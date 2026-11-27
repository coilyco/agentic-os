// What a host is for, the contract's `host_kind` (COI-3767). Named machine kind here
// because `Host.kind` already says daemon or mock. See references/host-kind.md.

export type MachineKind = "personal" | "ephemeral" | "durable";

export const MACHINE_KINDS: readonly MachineKind[] = ["personal", "ephemeral", "durable"];

export const MACHINE_KIND_LABEL: Record<MachineKind, string> = {
  personal: "Personal",
  ephemeral: "Ephemeral",
  durable: "Durable",
};

export const MACHINE_KIND_MEANING: Record<MachineKind, string> = {
  personal: "A desktop or laptop someone uses and closes. It may sleep.",
  ephemeral: "A batch machine that ends when its work is done.",
  durable: "An always-on server.",
};

/** A known kind, or undefined for anything else, absent included. */
export function parseMachineKind(raw: unknown): MachineKind | undefined {
  return typeof raw === "string" && (MACHINE_KINDS as readonly string[]).includes(raw) ? (raw as MachineKind) : undefined;
}
