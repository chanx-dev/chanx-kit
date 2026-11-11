import { describe, expect, it } from 'vitest';

import { InMemoryReplayStore, SerialJobs } from '..';

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

describe('SerialJobs', () => {
  it('runs jobs for a key in order, one at a time', async () => {
    const jobs = new SerialJobs();
    const seen: string[] = [];
    for (const name of ['a', 'b', 'c']) {
      jobs.submit('k', async () => {
        seen.push(`${name}-start`);
        await sleep(5);
        seen.push(`${name}-end`);
      });
    }
    await jobs.wait('k');
    expect(seen).toEqual(['a-start', 'a-end', 'b-start', 'b-end', 'c-start', 'c-end']);
    expect(jobs.busy('k')).toBe(false);
  });

  it('runs keys independently', async () => {
    const jobs = new SerialJobs();
    let release!: () => void;
    jobs.submit('slow', () => new Promise<void>((r) => (release = r)));
    let fast = false;
    jobs.submit('fast', async () => void (fast = true));
    await jobs.wait('fast');
    expect(fast).toBe(true);
    release();
    await jobs.wait('slow');
  });

  it('cancel stops the current job and drops the queue', async () => {
    const jobs = new SerialJobs();
    const seen: string[] = [];
    let started!: () => void;
    const running = new Promise<void>((r) => (started = r));
    jobs.submit('k', (signal) => {
      started();
      return new Promise<void>((resolve) =>
        signal.addEventListener('abort', () => {
          seen.push('stopped');
          resolve();
        }),
      );
    });
    jobs.submit('k', async () => void seen.push('queued ran'));
    await running;
    expect(await jobs.cancel('k')).toBe(true);
    await sleep(10);
    expect(seen).toEqual(['stopped']);
    expect(await jobs.cancel('k')).toBe(false);
  });

  it('a failing job does not stop the next', async () => {
    const jobs = new SerialJobs();
    const seen: string[] = [];
    const error = console.error;
    console.error = () => {};
    jobs.submit('k', async () => {
      throw new Error('boom');
    });
    jobs.submit('k', async () => void seen.push('next'));
    await jobs.wait('k');
    console.error = error;
    expect(seen).toEqual(['next']);
  });
});

describe('InMemoryReplayStore', () => {
  it('numbers from 1 and restarts after clear', async () => {
    const store = new InMemoryReplayStore<string>();
    await store.append('k', 'a');
    await store.append('k', 'b');
    expect(await store.replay('k')).toEqual([[1, 'a'], [2, 'b']]);
    await store.clear('k');
    expect(await store.append('k', 'c')).toBe(1);
  });

  it('is bounded', async () => {
    const store = new InMemoryReplayStore<string>(2);
    for (const m of ['a', 'b', 'c']) await store.append('k', m);
    expect(await store.replay('k')).toEqual([[2, 'b'], [3, 'c']]);
  });
});
