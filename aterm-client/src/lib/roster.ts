export interface Seat {
  key: string;
  harness: string;
  name: string;
  tier: string;
}

export interface Role {
  slug: string;
  displayName: string;
  purpose: string;
  color: string;
  identity: string;
  launchable: boolean;
  seats: Seat[];
}

export class RosterError extends Error {}

const FORMAT = "aterm.roster.v1";
const HEX = /^#[0-9a-f]{6}$/i;

function record(value: unknown, where: string): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new RosterError(`${where} is not an object`);
  }
  return value as Record<string, unknown>;
}

function text(value: unknown, where: string): string {
  if (typeof value !== "string") throw new RosterError(`${where} is not a string`);
  return value;
}

/** Decodes `aterm --list --json`, refusing any other contract instead of guessing. */
export function parseRoster(input: unknown): Role[] {
  const root = record(input, "roster");
  if (root.format !== FORMAT) {
    throw new RosterError(`expected format ${FORMAT}, got ${String(root.format)}`);
  }
  if (!Array.isArray(root.roles)) throw new RosterError("roster.roles is not a list");
  return root.roles.map((raw, index) => {
    const role = record(raw, `roles[${index}]`);
    const slug = text(role.slug, `roles[${index}].slug`);
    const color = text(role.favorite_color, `${slug}.favorite_color`);
    if (!HEX.test(color)) throw new RosterError(`${slug}.favorite_color is not #rrggbb`);
    const seats = Array.isArray(role.seats) ? role.seats : [];
    return {
      slug,
      displayName: text(role.display_name, `${slug}.display_name`),
      purpose: text(role.purpose ?? "", `${slug}.purpose`),
      color: color.toLowerCase(),
      identity: text(record(role.identity, `${slug}.identity`).name, `${slug}.identity.name`),
      launchable: role.launchable === true,
      seats: seats.map((rawSeat, seatIndex) => {
        const seat = record(rawSeat, `${slug}.seats[${seatIndex}]`);
        return {
          key: text(seat.key, `${slug}.seats[${seatIndex}].key`),
          harness: text(seat.harness, `${slug}.seats[${seatIndex}].harness`),
          name: text(seat.name, `${slug}.seats[${seatIndex}].name`),
          tier: typeof seat.tier === "string" ? seat.tier : "",
        };
      }),
    };
  });
}
