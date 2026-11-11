import { describe, expect, it } from 'vitest';

import { Base64Data, ChunkSequencer, MediaIntake, toBase64 } from '..';

const b = (s: string) => Buffer.from(s);

describe('base64', () => {
  it('travels as standard base64', () => {
    expect(toBase64(Buffer.from([0xff, 0x00, 0x10]))).toBe('/wAQ');
    expect(Base64Data.parse('/wAQ')).toEqual(Buffer.from([0xff, 0x00, 0x10]));
  });

  it('rejects invalid base64', () => {
    expect(Base64Data.safeParse('not base64!').success).toBe(false);
  });
});

describe('ChunkSequencer', () => {
  it('passes in-order chunks straight through', () => {
    const s = new ChunkSequencer<string>();
    expect(s.push(0, 'a')).toEqual([['a'], []]);
    expect(s.push(1, 'b')).toEqual([['b'], []]);
  });

  it('holds an early chunk for the one before it', () => {
    const s = new ChunkSequencer<string>();
    expect(s.push(1, 'b')).toEqual([[], []]);
    expect(s.push(0, 'a')).toEqual([['a', 'b'], []]);
  });

  it('drops duplicates and stale chunks', () => {
    const s = new ChunkSequencer<string>();
    s.push(0, 'a');
    expect(s.push(0, 'a')).toEqual([[], []]);
    s.push(2, 'c');
    expect(s.push(2, 'c')).toEqual([[], []]);
  });

  it('turns a missing chunk into a gap once the window fills', () => {
    const s = new ChunkSequencer<string>(2);
    s.push(0, 'a');
    expect(s.push(2, 'c')).toEqual([[], []]);
    expect(s.push(3, 'd')).toEqual([[], []]);
    expect(s.push(4, 'e')).toEqual([['c', 'd', 'e'], [{ start: 1, end: 2 }]]);
  });

  it('flushes what is held and names the gaps', () => {
    const s = new ChunkSequencer<string>();
    s.push(0, 'a');
    s.push(3, 'd');
    expect(s.flush()).toEqual([['d'], [{ start: 1, end: 3 }]]);
  });
});

describe('MediaIntake', () => {
  it('feeds in order, then ends', async () => {
    const seen: string[] = [];
    let ended = false;
    const intake = new MediaIntake<Buffer>((c) => void seen.push(c.toString()), {
      onEnd: () => void (ended = true),
    });
    intake.start();
    for (const c of ['1', '2', '3']) intake.feed(b(c));
    intake.end();
    await intake.wait();
    expect(seen).toEqual(['1', '2', '3']);
    expect(ended).toBe(true);
  });

  it('drops the oldest when full', async () => {
    const seen: string[] = [];
    let release!: () => void;
    const released = new Promise<void>((r) => (release = r));
    const intake = new MediaIntake<Buffer>(
      async (c) => {
        await released;
        seen.push(c.toString());
      },
      { onEnd: () => {}, maxChunks: 2 },
    );
    intake.start();
    intake.feed(b('1'));
    await new Promise((r) => setTimeout(r, 0)); // the sink now holds 1
    for (const c of ['2', '3', '4']) intake.feed(b(c));
    intake.end();
    release();
    await intake.wait();
    expect(intake.dropped).toBe(2);
    expect(seen).toEqual(['1', '4']);
  });

  it('sends keepalives while quiet, then goes idle', async () => {
    const quiet: string[] = [];
    const intake = new MediaIntake<Buffer>(() => {}, {
      onEnd: () => {},
      onQuiet: (q) => void quiet.push(q),
      keepaliveSeconds: 0.02,
      idleSeconds: 0.07,
    });
    intake.start();
    await intake.wait();
    expect(quiet.at(-1)).toBe('idle');
    expect(quiet.filter((q) => q === 'keepalive').length).toBeGreaterThanOrEqual(2);
  });

  it('can be stopped from inside onQuiet', async () => {
    let intake!: MediaIntake<Buffer>;
    intake = new MediaIntake<Buffer>(() => {}, {
      onEnd: () => {},
      onQuiet: () => intake.stop(),
      keepaliveSeconds: 0.01,
    });
    intake.start();
    await intake.wait();
    expect(intake.running).toBe(false);
  });
});
