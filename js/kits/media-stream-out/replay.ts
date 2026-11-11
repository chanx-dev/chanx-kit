/** One buffered stream per key; sequence numbers restart when it is cleared. */
export interface ReplayStore<M = unknown> {
  /** Buffer a message and return its sequence number, from 1. */
  append(key: string, message: M): Promise<number>;
  replay(key: string): Promise<[number, M][]>;
  clear(key: string): Promise<void>;
}

/** Process-local and bounded. Implement `ReplayStore` against Redis to share it. */
export class InMemoryReplayStore<M = unknown> implements ReplayStore<M> {
  private streams = new Map<string, [number, M][]>();

  constructor(private readonly maxMessages = 3000) {}

  async append(key: string, message: M) {
    const buffered = this.streams.get(key) ?? [];
    const seq = buffered.length > 0 ? buffered.at(-1)![0] + 1 : 1;
    // Dropping the oldest costs a late joiner the start, which it sees as a gap.
    if (buffered.length >= this.maxMessages) buffered.shift();
    buffered.push([seq, message]);
    this.streams.set(key, buffered);
    return seq;
  }

  async replay(key: string) {
    return [...(this.streams.get(key) ?? [])];
  }

  async clear(key: string) {
    this.streams.delete(key);
  }
}
