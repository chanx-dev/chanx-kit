import type { BaseEvent } from '@ag-ui/core';

/** One buffered run per thread; sequence numbers restart with each run. */
export interface RunEventStore {
  /** Buffer an event and return its sequence number, counting from 1. */
  append(threadId: string, event: BaseEvent): Promise<number>;
  replay(threadId: string): Promise<[number, BaseEvent][]>;
  clear(threadId: string): Promise<void>;
}

/** Which run, if any, holds each thread. */
export interface ActiveRunStore {
  /** Claim the thread for `runId`; the run already holding it, or undefined on success. */
  begin(threadId: string, runId: string): Promise<string | undefined>;
  /** Release the thread, if `runId` still holds it. */
  end(threadId: string, runId: string): Promise<void>;
}

/** Process-local and bounded. */
export class InMemoryRunEventStore implements RunEventStore {
  private readonly runs = new Map<string, [number, BaseEvent][]>();

  constructor(private readonly maxEventsPerRun = 2000) {}

  async append(threadId: string, event: BaseEvent) {
    const buffered = this.runs.get(threadId) ?? [];
    this.runs.set(threadId, buffered);
    const seq = (buffered.at(-1)?.[0] ?? 0) + 1;
    if (buffered.length >= this.maxEventsPerRun) buffered.shift();
    buffered.push([seq, event]);
    return seq;
  }

  async replay(threadId: string) {
    return [...(this.runs.get(threadId) ?? [])];
  }

  async clear(threadId: string) {
    this.runs.delete(threadId);
  }
}

/** Process-local. */
export class InMemoryActiveRunStore implements ActiveRunStore {
  private readonly active = new Map<string, string>();

  async begin(threadId: string, runId: string) {
    const holder = this.active.get(threadId);
    if (holder === undefined) this.active.set(threadId, runId);
    return holder;
  }

  async end(threadId: string, runId: string) {
    if (this.active.get(threadId) === runId) this.active.delete(threadId);
  }
}
