// On-device record of what a tap did (COI-3334, COI-3243). Off unless ?taptrace is set.
// Never holds seat content, an address or a token. See the tooling-aterm-client skill.

export const TRACE_FLAG = "taptrace";
/** What one paste into a message carries, header included. */
export const TRACE_TEXT_MAX = 7000;
const MAX_EVENTS = 400;

export type Renderer = "daemon-card" | "screen-card" | "terminal";

export interface TraceEvent {
  /** Milliseconds since the first event, one decimal. */
  t: number;
  type: string;
  target: string;
  x?: number;
  y?: number;
  renderer: Renderer;
  /** A short fixed word, never content. */
  detail?: string;
}

/** The only part of an element the record reads. A test passes a plain object. */
export interface Shape {
  tagName?: string;
  id?: string;
  className?: unknown;
  disabled?: boolean;
  parentElement?: Shape | null;
  getAttribute?(name: string): string | null;
  closest?(selector: string): Shape | null;
  /** The position among its siblings of a list item's button. */
  index?: number;
  /** The button or link a tap on an inner element lands in. */
  within?: Shape | null;
}

export function traceWanted(search: string): boolean {
  const value = new URLSearchParams(search).get(TRACE_FLAG);
  return value !== null && value !== "0" && value !== "off";
}

// Svelte's scoped class names carry nothing a person can use and change per build.
const SCOPED = /^svelte-/;
const NAME = /^[a-z][a-z0-9-]{0,24}$/;

/** A target as tag, id, class words and state. Never its text, value or label. */
export function describe(shape: Shape | null | undefined): string {
  if (!shape?.tagName) return "none";
  const tag = shape.tagName.toLowerCase();
  const classes = String(typeof shape.className === "string" ? shape.className : "")
    .split(/\s+/)
    .filter((word) => NAME.test(word) && !SCOPED.test(word))
    .slice(0, 3);
  const id = shape.id && NAME.test(shape.id) ? `#${shape.id}` : "";
  const place = typeof shape.index === "number" ? `[${shape.index}]` : "";
  const state = shape.disabled ? ":disabled" : "";
  const self = `${tag}${id}${classes.length ? `.${classes.join(".")}` : ""}${place}${state}`;
  return shape.within ? `${self}>${describe(shape.within)}` : self;
}

/** An option is told by its place in the card, never its label. */
function shapeOf(element: Element | null, inner = true): Shape | null {
  if (!element) return null;
  const owner = inner && !element.matches("button, a, input, textarea, [role=tab]") ? element.closest("button, a, [role=tab]") : null;
  const item = element.closest("li");
  const index = item?.parentElement ? Array.prototype.indexOf.call(item.parentElement.children, item) : undefined;
  return {
    tagName: element.tagName,
    id: element.id,
    className: element.getAttribute("class") ?? "",
    disabled: (element as HTMLButtonElement).disabled === true,
    ...(index !== undefined && index >= 0 && element.matches("button.option") ? { index } : {}),
    ...(owner ? { within: shapeOf(owner, false) } : {}),
  };
}

export function rendererOf(doc: Pick<Document, "querySelector">): Renderer {
  const card = doc.querySelector("section.card");
  const kind = card?.parentElement?.getAttribute("data-card");
  return kind === "daemon" ? "daemon-card" : card ? "screen-card" : "terminal";
}

const EVENTS = [
  "pointerdown",
  "pointerup",
  "pointercancel",
  "touchstart",
  "touchend",
  "touchcancel",
  "mouseover",
  "mousedown",
  "mouseup",
  "click",
  "focusin",
  "focusout",
] as const;

let events: TraceEvent[] = [];
let started = 0;
let on = false;
let seen: () => Renderer = () => "terminal";
const listeners = new Set<() => void>();

function add(type: string, target: string, extra: Partial<TraceEvent> = {}): void {
  const now = performance.now();
  if (events.length === 0) started = now;
  events.push({ t: Math.round((now - started) * 10) / 10, type, target, renderer: seen(), ...extra });
  if (events.length > MAX_EVENTS) events.shift();
  listeners.forEach((listener) => listener());
}

/** A fixed word the app says about itself. No-op when off. */
export function mark(type: string, detail?: string): void {
  if (!on) return;
  add(type, "app", detail ? { detail } : {});
}

export const records = (): readonly TraceEvent[] => events;
export const clear = (): void => {
  events = [];
  listeners.forEach((listener) => listener());
};
export const onChange = (listener: () => void): (() => void) => {
  listeners.add(listener);
  return () => listeners.delete(listener);
};

/** The record as lines. The oldest events go first when it will not fit. */
export function format(list: readonly TraceEvent[], header: string, max = TRACE_TEXT_MAX): string {
  const line = (event: TraceEvent): string =>
    `+${event.t.toFixed(1)} ${event.type} ${event.target}${event.x === undefined ? "" : ` @${event.x},${event.y}`} [${event.renderer}]${event.detail ? ` ${event.detail}` : ""}`;
  const lines = list.map(line);
  let dropped = 0;
  const size = (): number => header.length + lines.slice(dropped).reduce((sum, each) => sum + each.length + 1, 0) + 40;
  while (size() > max && dropped < lines.length) dropped += 1;
  return [header, ...(dropped ? [`(${dropped} earlier events dropped)`] : []), ...lines.slice(dropped)].join("\n");
}

/** Where the device is: viewport, visual viewport, pixel ratio, hover. No address. */
export function headerFor(win: Window, build: string, place: string): string {
  const view = win.visualViewport;
  const hover = typeof win.matchMedia === "function" && win.matchMedia("(hover: hover)").matches;
  return [
    `taptrace v1 build=${build} place=${place}`,
    `ua=${win.navigator.userAgent}`,
    `viewport=${win.innerWidth}x${win.innerHeight} visual=${Math.round(view?.width ?? 0)}x${Math.round(view?.height ?? 0)} dpr=${win.devicePixelRatio || 1} hover=${hover}`,
  ].join("\n");
}

/** Starts recording when the flag is set. Returns stop, or null when off. */
export function startTrace(win: Window & typeof globalThis): (() => void) | null {
  if (on || !traceWanted(win.location.search)) return null;
  on = true;
  events = [];
  seen = () => rendererOf(win.document);
  const quiet = (target: EventTarget | null): boolean => target instanceof Element && target.closest("[data-taptrace]") !== null;
  const handler = (event: Event): void => {
    try {
      if (quiet(event.target)) return;
      const touch = (event as TouchEvent).changedTouches?.[0];
      const point = touch ?? (event as MouseEvent);
      const hasPoint = typeof point.clientX === "number";
      add(event.type, describe(event.target instanceof Element ? shapeOf(event.target) : null), hasPoint ? { x: Math.round(point.clientX), y: Math.round(point.clientY) } : {});
    } catch {
      // A trace that fails must not become an error report.
    }
  };
  for (const type of EVENTS) win.document.addEventListener(type, handler, { capture: true, passive: true });
  // A card arriving or leaving is the other half of "dropped to the terminal".
  let card = win.document.querySelector("section.card") !== null;
  const watch = new win.MutationObserver(() => {
    try {
      const now = win.document.querySelector("section.card") !== null;
      if (now !== card) {
        card = now;
        add(now ? "card-shown" : "card-gone", "app");
      }
    } catch {
      // Same rule as above.
    }
  });
  watch.observe(win.document.body, { childList: true, subtree: true });
  // The on-screen keyboard shows as the visual viewport shrinking.
  const view = win.visualViewport;
  const resized = (): void => add("viewport", "app", { detail: `${Math.round(view?.width ?? 0)}x${Math.round(view?.height ?? 0)}` });
  view?.addEventListener("resize", resized);
  // Set only with the flag. The test bed reads it and nothing sends it.
  (win as unknown as { __tapTrace?: () => readonly TraceEvent[] }).__tapTrace = records;
  return () => {
    for (const type of EVENTS) win.document.removeEventListener(type, handler, { capture: true });
    watch.disconnect();
    view?.removeEventListener("resize", resized);
    delete (win as unknown as { __tapTrace?: unknown }).__tapTrace;
    on = false;
  };
}
