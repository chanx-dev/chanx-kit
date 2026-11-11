import { createServer } from 'node:net';
import type { AddressInfo, Socket } from 'node:net';

import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { deepgramSynthesize, deepgramSynthesizerTopic, speakUrl } from '../synthesizer';
import type { DeepgramSynthesizerOptions } from '../synthesizer';
import { KEY, fakeDeepgram, speak } from './fake';

const closers: (() => void)[] = [];
afterEach(() => closers.splice(0).forEach((close) => close()));

async function say(options: DeepgramSynthesizerOptions, text: string) {
  const topic = deepgramSynthesizerTopic(options);
  const chanx = createChanx({ consumers: [consumer('/ws', { name: 'voice', topics: [topic] })] });
  const c = connect(chanx, '/ws');
  await c.subscribe('speak:s');
  c.send({ action: 'speak', topic: 'speak:s', payload: { text } });
  return c;
}

async function local() {
  const fake = await fakeDeepgram(speak);
  closers.push(fake.close);
  return fake;
}

describe('deepgram synthesizer', () => {
  it('speaks text as whole frames', async () => {
    const { url, recording } = await local();
    const c = await say({ apiKey: KEY, url }, 'hi there');
    const end = (await c.receiveUntil('audio_end')).at(-1)!;

    expect((end.payload as { duration_ms: number }).duration_ms).toBe(160);
    expect(Object.fromEntries(recording.queries[0]!)).toEqual({
      model: 'aura-2-thalia-en',
      encoding: 'linear16',
      sample_rate: '24000',
    });
    await vi.waitFor(() => expect(recording.controls).toEqual(['Speak', 'Flush', 'Close']));
  });

  it('refuses a format Aura cannot make', () => {
    expect(() => speakUrl('aura', { encoding: 'pcm16', sample_rate: 22050, channels: 1 })).toThrow('mono PCM');
  });

  it('reports a wrong key on the utterance', async () => {
    const { url } = await local();
    const c = await say({ apiKey: 'nope', url }, 'hi');
    const error = (await c.receiveUntil('synthesizer_error')).at(-1)!.payload as { code: string; message: string };
    expect(error.code).toBe('provider_unavailable');
    expect(error.message).toContain('401');
  });
  it('reports a provider that never answers', async () => {
    const sockets: Socket[] = [];
    const silent = createServer((socket) => void sockets.push(socket));
    await new Promise<void>((r) => silent.listen(0, '127.0.0.1', r));
    closers.push(() => {
      sockets.forEach((socket) => socket.destroy());
      silent.close();
    });
    const host = `127.0.0.1:${(silent.address() as AddressInfo).port}`;
    const speech = deepgramSynthesize({ apiKey: 'k', url: `ws://${host}/{voice_id}`, connectTimeoutMs: 100 });
    const format = { encoding: 'pcm16' as const, sample_rate: 24000, channels: 1 };
    await expect(
      (async () => {
        for await (const _ of speech('hi', undefined, format, new AbortController().signal));
      })(),
    ).rejects.toThrow('did not answer');
  });
});
