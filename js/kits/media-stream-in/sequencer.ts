/** Chunks `start` up to `end` (exclusive) that never arrived. */
export interface Gap {
  start: number;
  end: number;
}

/** Releases chunks in index order and reports ones that never arrive as gaps. */
export class ChunkSequencer<T = Buffer> {
  expected = 0;
  private pending = new Map<number, T>();

  /** `window` chunks wait for a missing one before it counts as lost. */
  constructor(readonly window = 8) {}

  reset() {
    this.expected = 0;
    this.pending.clear();
  }

  push(index: number, data: T): [T[], Gap[]] {
    if (index < this.expected || this.pending.has(index)) return [[], []];
    this.pending.set(index, data);
    return this.release(false);
  }

  flush(): [T[], Gap[]] {
    return this.release(true);
  }

  private release(giveUp: boolean): [T[], Gap[]] {
    const ready: T[] = [];
    const gaps: Gap[] = [];
    while (this.pending.size > 0) {
      const next = this.pending.get(this.expected);
      if (next !== undefined) {
        this.pending.delete(this.expected);
        ready.push(next);
        this.expected += 1;
      } else if (giveUp || this.pending.size > this.window) {
        const following = Math.min(...this.pending.keys());
        gaps.push({ start: this.expected, end: following });
        this.expected = following;
      } else {
        break;
      }
    }
    return [ready, gaps];
  }
}
