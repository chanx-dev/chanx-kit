import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { deepgramTranscriberTopic, framesFrom } from '../transcriber';
import type { DeepgramTranscriberOptions } from '../transcriber';
import { KEY, fakeDeepgram, listen } from './fake';

const closers: (() => void)[] = [];
afterEach(() => closers.splice(0).forEach((close) => close()));

async function join(options: DeepgramTranscriberOptions) {
  const topic = deepgramTranscriberTopic(options);
  const chanx = createChanx({ consumers: [consumer('/ws', { name: 'voice', topics: [topic] })] });
  const c = connect(chanx, '/ws');
  await c.subscribe('transcribe:s');
  await c.receiveUntil('audio_config');
  return c;
}

const start = (c: Awaited<ReturnType<typeof join>>, language?: string) =>
  c.send({ action: 'audio_start', topic: 'transcribe:s', payload: { sample_rate: 24000, channels: 1, language } });

async function local() {
  const fake = await fakeDeepgram(listen);
  closers.push(fake.close);
  return fake;
}

describe('deepgram transcriber', () => {
  it('transcribes speech in the negotiated format', async () => {
    const { url, recording } = await local();
    const c = await join({ apiKey: KEY, url });
    start(c, 'vi');
    for (let index = 0; index < 5; index += 1) {
      c.send({ action: 'audio_chunk', topic: 'transcribe:s', payload: { index, data: Buffer.alloc(4800).toString('base64') } });
    }
    c.send({ action: 'audio_end', topic: 'transcribe:s' });
    const final = (await c.receiveUntil('transcript_final')).at(-1)!;
    await c.receiveUntil('utterance_end');

    expect((final.payload as { text: string }).text).toBe('hello world');
    expect(Object.fromEntries(recording.queries[0]!)).toMatchObject({
      encoding: 'linear16',
      sample_rate: '24000',
      channels: '1',
      interim_results: 'true',
      language: 'vi',
    });
    expect(recording.audioBytes).toBe(5 * 4800);
    await vi.waitFor(() => expect(recording.controls.slice(-2)).toEqual(['Finalize', 'CloseStream']));
  });

  it('reports a wrong key', async () => {
    const { url } = await local();
    const c = await join({ apiKey: 'nope', url });
    start(c);
    const error = (await c.receiveUntil('transcriber_error')).at(-1)!.payload as { code: string; message: string };
    expect(error.code).toBe('provider_unavailable');
    expect(error.message).toContain('401');
  });

  it('is unavailable on subscribe without a key', async () => {
    vi.stubEnv('DEEPGRAM_API_KEY', '');
    const topic = deepgramTranscriberTopic();
    const chanx = createChanx({ consumers: [consumer('/ws', { name: 'voice', topics: [topic] })] });
    const c = connect(chanx, '/ws');
    await c.subscribe('transcribe:s');
    const error = (await c.receiveUntil('transcriber_error')).at(-1)!.payload as { code: string };
    expect(error.code).toBe('provider_unavailable');
    vi.unstubAllEnvs();
  });

  it('maps results to frames', () => {
    const interim = {
      type: 'Results',
      is_final: false,
      start: 1,
      duration: 0.5,
      channel: { alternatives: [{ transcript: 'hi', confidence: 0.5 }] },
    };
    expect(framesFrom(interim).map((f) => f.type)).toEqual(['partial']);
    const final = framesFrom({ ...interim, is_final: true, speech_final: true });
    expect(final.map((f) => f.type)).toEqual(['final', 'utterance_end']);
    expect(final[0]).toMatchObject({ endMs: 1500 });
    expect(framesFrom({ ...interim, channel: { alternatives: [{ transcript: '' }] } })).toEqual([]);
  });
});
