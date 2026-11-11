import { createServer } from 'node:http';
import type { AddressInfo } from 'node:net';

import { afterEach, describe, expect, it } from 'vitest';

import { SynthesizerUnavailableError } from '../../audio-stream-out';
import type { OpenAISynthesizerOptions } from '../synthesizer';
import { openAISynthesize } from '../synthesizer';

const KEY = 'test-key';
const closers: (() => void)[] = [];
afterEach(() => closers.splice(0).forEach((close) => close()));

/** OpenAI's speech endpoint: 20 ms of PCM per input character. */
async function fakeSpeech() {
  const requests: Record<string, unknown>[] = [];
  const server = createServer(async (req, res) => {
    if (req.headers.authorization !== `Bearer ${KEY}`) {
      res.writeHead(401).end('{"error":{"message":"bad key"}}');
      return;
    }
    let raw = '';
    for await (const piece of req) raw += piece;
    const body = JSON.parse(raw);
    requests.push(body);
    res.writeHead(200, { 'content-type': 'audio/pcm' }).end(Buffer.alloc(((body.input.length * 24000) / 50) * 2, 5));
  });
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
  closers.push(() => server.close());
  return { url: `http://127.0.0.1:${(server.address() as AddressInfo).port}/v1/audio/speech`, requests };
}

async function collect(options: OpenAISynthesizerOptions, sampleRate = 24000) {
  const pieces: Uint8Array[] = [];
  const format = { encoding: 'pcm16' as const, sample_rate: sampleRate, channels: 1 };
  for await (const piece of openAISynthesize(options)('Hello', 'nova', format, new AbortController().signal)) {
    pieces.push(piece);
  }
  return Buffer.concat(pieces);
}

describe('openai synthesizer', () => {
  it('asks for raw PCM', async () => {
    const { url, requests } = await fakeSpeech();
    const audio = await collect({ apiKey: KEY, url, instructions: 'Speak warmly.' });
    expect(requests[0]).toEqual({
      model: 'gpt-4o-mini-tts',
      voice: 'nova',
      input: 'Hello',
      response_format: 'pcm',
      instructions: 'Speak warmly.',
    });
    expect(audio.length).toBe(2 * ((5 * 24000) / 50));
  });

  it('reports a wrong key as unavailable', async () => {
    const { url } = await fakeSpeech();
    await expect(collect({ apiKey: 'nope', url })).rejects.toThrow(SynthesizerUnavailableError);
    await expect(collect({ apiKey: 'nope', url })).rejects.toThrow('401');
  });

  it('offers only 24 kHz mono', async () => {
    await expect(collect({ apiKey: KEY }, 16000)).rejects.toThrow(SynthesizerUnavailableError);
  });
});
