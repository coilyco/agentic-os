import { describe, expect, it } from "vitest";
import { driverLabel, isDriving, isStalled, keyParams, modifiersOf, mouseParams, paneSize, tabLabel, toPagePoint, type SharedBrowser } from "./screencast";

const metadata = { deviceWidth: 1280, deviceHeight: 800, offsetTop: 0, pageScaleFactor: 1 };
const none = { altKey: false, ctrlKey: false, metaKey: false, shiftKey: false };

describe("toPagePoint", () => {
  it("scales a point on a half-size frame back to page pixels", () => {
    expect(toPagePoint(320, 200, { left: 0, top: 0, width: 640, height: 400 }, metadata)).toEqual({ x: 640, y: 400 });
  });

  it("accounts for the letterbox of a contained frame", () => {
    // A 640x640 box draws the 1280x800 frame 400 tall, centred 120 down.
    const box = { left: 10, top: 10, width: 640, height: 640 };
    expect(toPagePoint(10, 130, box, metadata)).toEqual({ x: 0, y: 0 });
    expect(toPagePoint(10, 20, box, metadata)).toBeNull();
  });

  it("maps nothing before the first frame has a size", () => {
    expect(toPagePoint(1, 1, { left: 0, top: 0, width: 10, height: 10 }, { ...metadata, deviceWidth: 0 })).toBeNull();
  });
});

describe("paneSize", () => {
  it("rounds the pane to whole CSS pixels", () => {
    expect(paneSize({ width: 319.6, height: 640.2 })).toEqual({ width: 320, height: 640 });
  });

  it("gives no size for a pane that has no width, such as a hidden tab", () => {
    expect(paneSize(undefined)).toBeUndefined();
    expect(paneSize({ width: 0, height: 600 })).toBeUndefined();
    expect(paneSize({ width: 320, height: 0.4 })).toBeUndefined();
  });

  it("holds a squeezed pane to the height the stage never goes below", () => {
    expect(paneSize({ width: 288, height: 0 }, 240)).toEqual({ width: 288, height: 240 });
    expect(paneSize({ width: 288, height: 500 }, 240)).toEqual({ width: 288, height: 500 });
  });
});

describe("input params", () => {
  it("packs modifiers the way CDP counts them", () => {
    expect(modifiersOf({ altKey: true, ctrlKey: false, metaKey: true, shiftKey: true })).toBe(13);
  });

  it("names the button on a press and none on a move", () => {
    const event = { ...none, button: 2, buttons: 2, detail: 1 };
    expect(mouseParams("mousePressed", { x: 1, y: 2 }, event)).toMatchObject({ type: "mousePressed", button: "right", clickCount: 1 });
    expect(mouseParams("mouseMoved", { x: 1, y: 2 }, event)).toMatchObject({ button: "none", clickCount: 0 });
  });

  it("gives a printable key its text and a shortcut none", () => {
    expect(keyParams("keyDown", { ...none, key: "a", code: "KeyA", keyCode: 65 })).toMatchObject({ type: "keyDown", text: "a" });
    expect(keyParams("keyDown", { ...none, metaKey: true, key: "c", code: "KeyC", keyCode: 67 })).toMatchObject({ type: "rawKeyDown" });
    expect(keyParams("keyDown", { ...none, key: "Enter", code: "Enter", keyCode: 13 })).toMatchObject({ text: "\r" });
    expect(keyParams("keyUp", { ...none, key: "a", code: "KeyA", keyCode: 65 })).not.toHaveProperty("text");
  });
});

describe("isDriving", () => {
  const base: SharedBrowser = { session: "s", state: "live", driver: "person", heldHere: true, url: "", title: "" };

  it("drives only from the screen that took control", () => {
    expect(isDriving(base)).toBe(true);
    expect(isDriving({ ...base, heldHere: false, holder: "phone" })).toBe(false);
    expect(isDriving({ ...base, driver: "agent" })).toBe(false);
    expect(isDriving({ ...base, state: "closed" })).toBe(false);
    expect(isDriving(undefined)).toBe(false);
  });
});

describe("browser state", () => {
  const live: SharedBrowser = { session: "s", state: "live", driver: "agent", heldHere: false, url: "", title: "", frame: { src: "", metadata, at: 1000, seq: 1 } };

  it("calls a live stream stalled after five quiet seconds", () => {
    expect(isStalled(live, 5999)).toBe(false);
    expect(isStalled(live, 6001)).toBe(true);
    expect(isStalled({ ...live, state: "none" }, 60_000)).toBe(false);
  });

  it("says who is driving", () => {
    expect(driverLabel(live, "Frog-Ox")).toBe("Frog-Ox is driving.");
    expect(driverLabel({ ...live, driver: "person", heldHere: true }, "Frog-Ox")).toMatch(/^You have control/);
    expect(driverLabel({ ...live, driver: "person", holder: "phone" }, "Frog-Ox")).toMatch(/^Another screen has control/);
    expect(driverLabel({ ...live, state: "closed", reason: "the seat exited" }, "Frog-Ox")).toBe("The browser closed: the seat exited");
  });
});

describe("tabLabel", () => {
  const live = { session: "s", state: "live", driver: "agent", url: "", title: "", heldHere: false } as const;

  it("names the followed tab among the open ones", () => {
    expect(tabLabel({ ...live, tab: 2, tabs: 3 })).toBe("Tab 2 of 3");
  });

  it("says nothing for a single page, a daemon that sends no numbers, or a browser that is not live", () => {
    expect(tabLabel({ ...live, tab: 1, tabs: 1 })).toBeUndefined();
    expect(tabLabel(live)).toBeUndefined();
    expect(tabLabel({ ...live, state: "closed", tab: 2, tabs: 3 })).toBeUndefined();
  });
});
