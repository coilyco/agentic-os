// Light, dark or follow the device (COI-3554). Remembered on this device, not per seat.
const KEY = "aterm.theme.v1";

export type ThemeChoice = "system" | "light" | "dark";
export type Theme = "light" | "dark";

export const THEME_CHOICES: { value: ThemeChoice; label: string }[] = [
  { value: "system", label: "System" },
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
];

/** The stored choice, or System when none is stored or the value is unknown. */
export function loadThemeChoice(): ThemeChoice {
  try {
    const stored = localStorage.getItem(KEY);
    return THEME_CHOICES.find((each) => each.value === stored)?.value ?? "system";
  } catch {
    return "system";
  }
}

export function storeThemeChoice(choice: ThemeChoice): void {
  try {
    localStorage.setItem(KEY, choice);
  } catch {
    // Storage can be off (a private window). The choice holds until reload.
  }
}

/** System follows the device. A device that says nothing reads as dark, as before. */
export function resolveTheme(choice: ThemeChoice, deviceIsLight: boolean): Theme {
  return choice === "system" ? (deviceIsLight ? "light" : "dark") : choice;
}

export const deviceQuery = "(prefers-color-scheme: light)";
