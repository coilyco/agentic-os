// The accent for painters that take hex only, such as xterm's find highlights (COI-3510).

const HEX = /^#([0-9a-f]{6})$/i;

/** `amount` of `top` over `base`, both #rrggbb, as #rrggbb. */
export function mixHex(top: string, base: string, amount: number): string {
  const a = HEX.exec(top)?.[1];
  const b = HEX.exec(base)?.[1];
  if (!a || !b) return base;
  const channel = (at: number) => {
    const over = parseInt(a.slice(at, at + 2), 16);
    const under = parseInt(b.slice(at, at + 2), 16);
    return Math.round(over * amount + under * (1 - amount))
      .toString(16)
      .padStart(2, "0");
  };
  return `#${channel(0)}${channel(2)}${channel(4)}`;
}

/** Find highlights from the accent token over the ground, so a retune carries over. */
export function findColours(accent: string, ground: string) {
  return {
    matchBackground: mixHex(accent, ground, 0.22),
    matchOverviewRuler: mixHex(accent, ground, 0.5),
    activeMatchBackground: mixHex(accent, ground, 0.42),
    activeMatchBorder: accent,
    activeMatchColorOverviewRuler: accent,
  };
}

/** A computed colour as #rrggbb. A derived one computes to `color(srgb ...)`. */
export function hexOfComputed(painted: string): string | undefined {
  const parts = /^(rgba?|color)\(\s*(?:srgb\s+)?([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)/.exec(painted);
  if (!parts) return undefined;
  const scale = parts[1] === "color" ? 255 : 1;
  const channels = parts.slice(2, 5).map((part) => Math.min(255, Math.round(Number(part) * scale)));
  return `#${channels.map((channel) => channel.toString(16).padStart(2, "0")).join("")}`;
}

/** Any CSS colour, `var(--token)` included, as #rrggbb for hex-only painters. */
export function resolveColour(value: string): string {
  if (typeof document === "undefined") return value;
  const probe = document.createElement("span");
  probe.style.color = value;
  document.documentElement.append(probe);
  const painted = getComputedStyle(probe).color;
  probe.remove();
  return hexOfComputed(painted) ?? value;
}

const ANSI = ["black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"] as const;

/** The 16 ANSI colours from the --ansi-* tokens, in the names xterm's theme takes. */
export function ansiTheme(): Record<string, string> {
  const theme: Record<string, string> = {};
  for (const name of ANSI) {
    theme[name] = resolveColour(`var(--ansi-${name})`);
    theme[`bright${name[0]!.toUpperCase()}${name.slice(1)}`] = resolveColour(`var(--ansi-bright-${name})`);
  }
  return theme;
}
