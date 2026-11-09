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

/** The token's current value, with the shipped green when the page has none to read. */
export function readToken(name: string, fallback: string): string {
  if (typeof document === "undefined") return fallback;
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return HEX.test(value) ? value : fallback;
}
