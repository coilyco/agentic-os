// Why an address did not open, shown with the opened-page cards. It serves a card,
// a terminal link and an MCP view's link alike.
import { openExternal } from "./open-external";

export const linkNotice = $state({ text: "" });

/** Opens an address in the device's browser. A failure's reason stays on screen. */
export async function openLink(url: string): Promise<boolean> {
  const result = await openExternal(url);
  linkNotice.text = result.ok ? "" : result.message;
  return result.ok;
}

export function clearLinkNotice(): void {
  linkNotice.text = "";
}
