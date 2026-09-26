// The shared browser as the client sees it: CDP screencast frames in, CDP
// Input.dispatch* params out. Shapes follow the Chrome DevTools Protocol.

export type BrowserState = "none" | "idle" | "live" | "closed";
export type Driver = "agent" | "person";

/** CDP Page.ScreencastFrameMetadata, the part the client uses. */
export interface FrameMetadata {
  deviceWidth: number;
  deviceHeight: number;
  offsetTop: number;
  pageScaleFactor: number;
}

export interface SharedBrowser {
  session: string;
  state: BrowserState;
  driver: Driver;
  url: string;
  title: string;
  reason?: string;
  frame?: { src: string; metadata: FrameMetadata; at: number };
}

export type InputKind = "mouse" | "wheel" | "key" | "text";

/** A frame older than this while the page is live means the stream stalled. */
export const STALL_MS = 5000;

export function isStalled(browser: SharedBrowser, now: number): boolean {
  return browser.state === "live" && browser.frame !== undefined && now - browser.frame.at > STALL_MS;
}

interface Box {
  left: number;
  top: number;
  width: number;
  height: number;
}

// Where a point on the drawn frame lands in the page, in CSS pixels. The frame
// is drawn contained and centered, so the letterbox maps to nothing.
export function toPagePoint(clientX: number, clientY: number, box: Box, metadata: FrameMetadata): { x: number; y: number } | null {
  const { deviceWidth, deviceHeight } = metadata;
  if (!deviceWidth || !deviceHeight || !box.width || !box.height) return null;
  const scale = Math.min(box.width / deviceWidth, box.height / deviceHeight);
  const left = box.left + (box.width - deviceWidth * scale) / 2;
  const top = box.top + (box.height - deviceHeight * scale) / 2;
  const x = (clientX - left) / scale;
  const y = (clientY - top) / scale - metadata.offsetTop;
  if (x < 0 || y < 0 || x > deviceWidth || y > deviceHeight) return null;
  return { x: Math.round(x), y: Math.round(y) };
}

interface Modifiers {
  altKey: boolean;
  ctrlKey: boolean;
  metaKey: boolean;
  shiftKey: boolean;
}

/** CDP's modifier bit field: Alt 1, Ctrl 2, Meta 4, Shift 8. */
export function modifiersOf(event: Modifiers): number {
  return (event.altKey ? 1 : 0) | (event.ctrlKey ? 2 : 0) | (event.metaKey ? 4 : 0) | (event.shiftKey ? 8 : 0);
}

const BUTTONS = ["left", "middle", "right"] as const;

export function mouseParams(
  type: "mousePressed" | "mouseReleased" | "mouseMoved",
  point: { x: number; y: number },
  event: Modifiers & { button: number; buttons: number; detail: number },
): Record<string, unknown> {
  return {
    type,
    x: point.x,
    y: point.y,
    button: type === "mouseMoved" ? "none" : (BUTTONS[event.button] ?? "none"),
    buttons: event.buttons,
    clickCount: type === "mouseMoved" ? 0 : Math.max(1, event.detail),
    modifiers: modifiersOf(event),
  };
}

export function wheelParams(point: { x: number; y: number }, event: Modifiers & { deltaX: number; deltaY: number }): Record<string, unknown> {
  return { type: "mouseWheel", x: point.x, y: point.y, deltaX: event.deltaX, deltaY: event.deltaY, modifiers: modifiersOf(event) };
}

// A key as CDP wants it. A printable key carries its text on keyDown, so the
// page gets the key event and the character, as a real keyboard gives.
export function keyParams(type: "keyDown" | "keyUp", event: Modifiers & { key: string; code: string; keyCode: number }): Record<string, unknown> {
  const printable = event.key.length === 1 && !event.ctrlKey && !event.metaKey;
  const text = event.key === "Enter" ? "\r" : printable ? event.key : undefined;
  return {
    type: type === "keyDown" && text ? "keyDown" : type === "keyDown" ? "rawKeyDown" : "keyUp",
    key: event.key,
    code: event.code,
    windowsVirtualKeyCode: event.keyCode,
    modifiers: modifiersOf(event),
    ...(type === "keyDown" && text ? { text, unmodifiedText: text } : {}),
  };
}

/** What a person reads above the page, by who holds it. */
export function driverLabel(browser: SharedBrowser, identity: string): string {
  if (browser.state === "closed") return browser.reason ? `The browser closed: ${browser.reason}` : "The browser closed.";
  if (browser.driver === "person") return `You have control. ${identity} waits until you hand it back.`;
  return `${identity} is driving.`;
}
