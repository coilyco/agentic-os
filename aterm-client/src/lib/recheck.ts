import { backoffDelay } from "./backoff";

/** Asks again on the hour of backoff until `ask` says yes. A host that never answered
 * has no socket to redial, so this is its way back without a tap (COI-3352). */
export class Recheck {
  private attempt = 0;
  private timer?: ReturnType<typeof setTimeout>;
  private running = false;
  private stopped = false;

  constructor(private ask: () => Promise<boolean>) {}

  /** Begins the schedule. A schedule already under way keeps its place. */
  start(): void {
    if (this.stopped || this.timer !== undefined || this.running) return;
    this.schedule();
  }

  /** The person came back or the network did: ask now, and start the hour over. */
  wake(): void {
    if (this.stopped || this.running) return;
    clearTimeout(this.timer);
    this.timer = undefined;
    this.attempt = 0;
    void this.run();
  }

  stop(): void {
    this.stopped = true;
    clearTimeout(this.timer);
    this.timer = undefined;
  }

  private schedule(): void {
    const delay = backoffDelay(this.attempt);
    // The hour is spent. Only a wake asks again.
    if (delay === null) return;
    this.attempt += 1;
    this.timer = setTimeout(() => void this.run(), delay);
  }

  private async run(): Promise<void> {
    this.timer = undefined;
    this.running = true;
    let answered = false;
    try {
      answered = await this.ask();
    } catch {
      // A failed ask is the ordinary case here, not an error to report.
    }
    this.running = false;
    if (answered) this.stop();
    else if (!this.stopped) this.schedule();
  }
}
