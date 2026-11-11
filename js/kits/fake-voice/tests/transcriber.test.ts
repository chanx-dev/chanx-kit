import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { describe, expect, it } from 'vitest';

import { FakeTranscriberStream, fakeTranscriberTopic } from '../transcriber';

/** `ms` of audio loud enough to count as speech, at 48 bytes per ms. */
const loud = (ms: number) => {
  const b = Buffer.alloc(ms * 48);
  for (let i = 0; i < b.length; i += 2) b.writeInt16LE(4000, i);
  return b;
};
const quiet = (ms: number) => Buffer.alloc(ms * 48);

async function described(script: string[], wordsPerFinal: number, ...chunks: Buffer[]) {
  const stream = new FakeTranscriberStream(
    { sample_rate: 24000, channels: 1 },
    { script, msPerWord: 100, wordsPerFinal, speechLevel: 500, silenceMs: 300 },
  );
  for (const chunk of chunks) await stream.send(chunk);
  await stream.finish();
  const out: string[] = [];
  for await (const frame of stream.frames()) {
    switch (frame.type) {
      case 'speech_started':
        out.push('started');
        break;
      case 'partial':
      case 'final':
        out.push(`${frame.type}:${frame.text}`);
        break;
      case 'utterance_end':
        out.push('end');
        break;
    }
  }
  return out;
}

describe('fake transcriber', () => {
  it('paces words by loud audio', async () => {
    expect(await described(['a', 'b', 'c'], 10, loud(250))).toEqual([
      'started',
      'partial:a',
      'partial:a b',
      'final:a b',
      'end',
    ]);
  });

  it('splits long speech into finals and repeats the script', async () => {
    const finals = (await described(['a', 'b'], 2, loud(500))).filter((d) => d.startsWith('final:'));
    expect(finals).toEqual(['final:a b', 'final:a b', 'final:a']);
  });

  it('ends an utterance on a pause, and speech starts another', async () => {
    const pause = [quiet(100), quiet(100), quiet(100), quiet(100)];
    expect(await described(['one', 'two', 'three', 'four'], 10, loud(100), loud(100), ...pause, loud(100), loud(100))).toEqual([
      'started',
      'partial:one',
      'partial:one two',
      'final:one two',
      'end',
      'started',
      'partial:three',
      'partial:three four',
      'final:three four',
      'end',
    ]);
  });

  it('does not end the utterance on a short pause', async () => {
    const got = await described(['a', 'b', 'c'], 10, loud(100), quiet(200), loud(100));
    expect(got.filter((d) => d === 'end')).toHaveLength(1);
  });

  it('hears nothing in silence', async () => {
    expect(await described(['a'], 10, quiet(1000))).toEqual([]);
  });

  it('transcribes speech through the topic', async () => {
    const topic = fakeTranscriberTopic({ script: ['hello', 'world'], msPerWord: 500, wordsPerFinal: 2 });
    const chanx = createChanx({ consumers: [consumer('/ws', { name: 'voice', topics: [topic] })] });
    const c = connect(chanx, '/ws');
    await c.subscribe('transcribe:s');
    await c.receiveUntil('audio_config');
    const at = 'transcribe:s';
    c.send({ action: 'audio_start', topic: at, payload: { encoding: 'pcm16', sample_rate: 24000, channels: 1 } });
    for (let index = 0; index < 10; index += 1) {
      c.send({ action: 'audio_chunk', topic: at, payload: { index, data: loud(100).toString('base64') } });
    }
    c.send({ action: 'audio_end', topic: at });
    const final = (await c.receiveUntil('transcript_final')).at(-1)!;
    expect((final.payload as { text: string }).text).toBe('hello world');
    await c.receiveUntil('utterance_end');
  });
});
