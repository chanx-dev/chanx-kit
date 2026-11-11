export type Job = (signal: AbortSignal) => Promise<void>;

interface Lane {
  queue: Job[];
  controller?: AbortController;
  done: Promise<void>;
}

/** Runs jobs one at a time per key, in order; `cancel` drops a key's current and queued jobs. */
export class SerialJobs {
  private lanes = new Map<string, Lane>();

  submit(key: string, job: Job) {
    const lane = this.lanes.get(key);
    if (lane) {
      lane.queue.push(job);
      return;
    }
    const created: Lane = { queue: [job], done: Promise.resolve() };
    this.lanes.set(key, created);
    created.done = this.work(key, created);
  }

  busy(key: string) {
    return this.lanes.has(key);
  }

  /** Stop the running job and drop the queued ones; true if anything stopped. */
  async cancel(key: string) {
    const lane = this.lanes.get(key);
    if (!lane) return false;
    this.lanes.delete(key);
    lane.queue = [];
    lane.controller?.abort();
    await lane.done;
    return true;
  }

  /** Until every job for `key` has run. Mostly for tests. */
  async wait(key: string) {
    await this.lanes.get(key)?.done;
  }

  private async work(key: string, lane: Lane) {
    // Start after submit returns, so the lane is registered first.
    await Promise.resolve();
    while (lane.queue.length > 0 && this.lanes.get(key) === lane) {
      const job = lane.queue.shift()!;
      lane.controller = new AbortController();
      try {
        await job(lane.controller.signal);
      } catch (error) {
        if (lane.controller.signal.aborted) return;
        console.error(`media job for ${key} failed`, error);
      }
      if (lane.controller.signal.aborted) return;
    }
    if (this.lanes.get(key) === lane) this.lanes.delete(key);
  }
}
