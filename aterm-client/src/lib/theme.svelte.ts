import dark from "../assets/treehouse-canopy.svg?url";
import light from "../assets/treehouse-canopy-light.svg?url";
import { loadThemeChoice, resolveTheme, storeThemeChoice, deviceQuery, type Theme, type ThemeChoice } from "./theme";

const query = typeof matchMedia === "function" ? matchMedia(deviceQuery) : null;

/** What a surface reads: the choice made, and the theme it resolves to now. */
export const theme = $state<{ choice: ThemeChoice; resolved: Theme }>({ choice: "system", resolved: "dark" });

/** The Android shell tints its bar icons from this, when it is there. */
interface Shell {
  set?: (theme: string) => void;
}

function paint(): void {
  const resolved = resolveTheme(theme.choice, query?.matches ?? false);
  const root = document.documentElement;
  // The attribute first, so a reader of `resolved` finds the tokens already moved.
  root.dataset.theme = resolved;
  theme.resolved = resolved;
  // The OS chrome reads --ground, so a retune of the token carries to the status bar.
  const ground = getComputedStyle(root).getPropertyValue("--ground").trim();
  document.querySelector('meta[name="theme-color"]')?.setAttribute("content", ground);
  try {
    (window as unknown as { TreehouseTheme?: Shell }).TreehouseTheme?.set?.(resolved);
  } catch {
    // A shell without the bridge keeps the system's own bar icons.
  }
}

export function setThemeChoice(choice: ThemeChoice): void {
  theme.choice = choice;
  storeThemeChoice(choice);
  paint();
}

/** Reads the stored choice, paints, and repaints when the device changes its mind. */
export function startTheme(): () => void {
  theme.choice = loadThemeChoice();
  paint();
  query?.addEventListener("change", paint);
  return () => query?.removeEventListener("change", paint);
}

/** The mark in the colours its website exports give for this ground (COI-3554). */
export const canopy = (): string => (theme.resolved === "light" ? light : dark);
