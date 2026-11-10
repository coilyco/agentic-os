// Which message in the Messages panel a transcript's peer item is (COI-3532).
// The two come from different frames and share no id, so sender and text decide.
import type { PeerRef } from "./transcript";
import type { PeerMessage } from "./protocol";

const flat = (text: string | null | undefined): string => (text ?? "").replace(/\s+/g, " ").trim();

/** The newest message from that sender with the peer's text, else the sender's newest. */
export function findPeerMessage(messages: readonly PeerMessage[], peer: PeerRef): PeerMessage | undefined {
  const sent = messages.filter((message) => !peer.from || message.from.identity === peer.from.identity);
  const want = flat(peer.text);
  const same = sent.filter((message) => {
    const text = flat(message.text);
    return want !== "" && text !== "" && (want.includes(text) || text.includes(want));
  });
  return (same.length > 0 ? same : sent).at(-1);
}
