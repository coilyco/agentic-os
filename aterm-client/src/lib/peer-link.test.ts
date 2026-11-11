import { describe, expect, it } from "vitest";
import { findPeerMessage } from "./peer-link";
import type { PeerMessage } from "./protocol";

const message = (id: string, identity: string, text: string | null): PeerMessage => ({ id, from: { role: "eng-platform", identity }, target: "frontend-eng", session: "s", state: "delivered", text } as PeerMessage);

describe("the message a transcript's peer item points at", () => {
  const peer = { id: "p", from: { role: "eng-platform", identity: "Beetle-Ox" }, text: "Is the frame fixed?" };

  it("is the one from that sender with that text", () => {
    const messages = [message("a", "Beetle-Ox", "Run the gate."), message("b", "Beetle-Ox", "Is the frame fixed?"), message("c", "Weevil-Ox", "Is the frame fixed?")];
    expect(findPeerMessage(messages, peer)?.id).toBe("b");
  });

  it("reads past spacing, and takes a text the host cut as a match for the longer one", () => {
    expect(findPeerMessage([message("a", "Beetle-Ox", "Is  the frame\nfixed?")], peer)?.id).toBe("a");
    expect(findPeerMessage([message("a", "Beetle-Ox", "Is the"), message("b", "Beetle-Ox", "Other.")], { ...peer, text: "Is the frame fixed? And more." })?.id, "the host's cut text is inside the peer's").toBe("a");
  });

  it("falls back to the sender's newest when no text matches", () => {
    const messages = [message("a", "Beetle-Ox", "Something else."), message("b", "Beetle-Ox", null)];
    expect(findPeerMessage(messages, peer)?.id).toBe("b");
  });

  it("takes the newest of two that match, and is undefined for a stranger", () => {
    expect(findPeerMessage([message("a", "Beetle-Ox", "Is the frame fixed?"), message("b", "Beetle-Ox", "Is the frame fixed?")], peer)?.id).toBe("b");
    expect(findPeerMessage([message("a", "Weevil-Ox", "hi")], peer)).toBeUndefined();
  });

  it("matches on text alone when the sender is not known", () => {
    expect(findPeerMessage([message("a", "Beetle-Ox", "Is the frame fixed?")], { id: "p", text: "Is the frame fixed?" })?.id).toBe("a");
  });
});
