// The last bytes a seat's terminal drew, kept in memory per seat (COI-3614). Reopening a
// seat writes them back at once and asks the daemon only for what came after, by offset.
// Nothing here is stored on the device.

export const TAIL_BYTES = 1024 * 1024;
export const MAX_SEATS = 6;

interface Tail {
  /** Names the run of the seat these bytes belong to, so a restarted seat starts over. */
  tag: string;
  chunks: Uint8Array[];
  bytes: number;
  /** The stream offset after the last byte held. */
  end: number;
}

export interface Noted {
  /** What the terminal still has to draw, or null when it has all of it already. */
  data: Uint8Array | null;
  /** The held bytes do not join this frame, so the screen is cleared first. */
  reset: boolean;
}

export class ScreenTails {
  private tails = new Map<string, Tail>();

  /** Starts keeping this seat, or starts over when it is a different run of it. */
  keep(name: string, tag: string): void {
    const held = this.tails.get(name);
    this.tails.delete(name);
    this.tails.set(name, held && held.tag === tag ? held : { tag, chunks: [], bytes: 0, end: -1 });
    for (const old of [...this.tails.keys()].slice(0, Math.max(0, this.tails.size - MAX_SEATS))) this.tails.delete(old);
  }

  /** Forgets what was drawn but keeps the seat, for a screen asked for whole. */
  clear(name: string): void {
    const held = this.tails.get(name);
    if (held) Object.assign(held, { chunks: [], bytes: 0, end: -1 });
  }

  /** The held bytes and the offset after them, or null when there are none. */
  resume(name: string): { bytes: Uint8Array; end: number } | null {
    const held = this.tails.get(name);
    if (!held || held.bytes === 0 || held.end < 0) return null;
    const bytes = new Uint8Array(held.bytes);
    let at = 0;
    for (const chunk of held.chunks) (bytes.set(chunk, at), (at += chunk.length));
    return { bytes, end: held.end };
  }

  /** Takes one output frame. A later frame with no offset ends keeping the seat. */
  note(name: string, data: Uint8Array, offset: number | undefined): Noted {
    const held = this.tails.get(name);
    if (!held) return { data, reset: false };
    // The daemon leaves `offset` out at 0, so a first frame without one starts at 0.
    const at = offset === undefined && held.end < 0 ? 0 : offset;
    if (at === undefined || !Number.isFinite(at)) {
      this.tails.delete(name);
      return { data, reset: false };
    }
    offset = at;
    const end = offset + data.length;
    if (held.end >= 0 && end <= held.end) return { data: null, reset: false };
    if (held.end < 0 || offset > held.end) {
      const reset = held.end >= 0;
      Object.assign(held, { chunks: [], bytes: 0 });
      this.append(held, data, end);
      return { data, reset };
    }
    const fresh = offset < held.end ? data.subarray(held.end - offset) : data;
    this.append(held, fresh, end);
    return { data: fresh, reset: false };
  }

  private append(held: Tail, data: Uint8Array, end: number): void {
    held.chunks.push(data);
    held.bytes += data.length;
    held.end = end;
    while (held.bytes > TAIL_BYTES) {
      const first = held.chunks[0]!;
      const over = held.bytes - TAIL_BYTES;
      if (first.length <= over) {
        held.chunks.shift();
        held.bytes -= first.length;
      } else {
        held.chunks[0] = first.subarray(over);
        held.bytes -= over;
      }
    }
  }
}
