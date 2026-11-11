import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { afterEach, describe, expect, it } from 'vitest';

import { ProviderUnavailableError } from '../../audio-stream-in';
import { elevenLabsTranscriberTopic, realtimeUrl } from '../transcriber';
import { KEY, fakeElevenLabs, scribe } from './fake';

const closers: (() => void)[] = [];
afterEach(() => closers.splice(0).forEach((close) => close()));

async function speakInto(language?: string) {
  const fake = await fakeElevenLabs(scribe);
  closers.push(fake.close);
  const topic = elevenLabsTranscriberTopic({ apiKey: KEY, url: fake.url });
  const chanx = createChanx({ consumers: [consumer('/ws', { name: 'voice', topics: [topic] })] });
  const c = connect(chanx, '/ws');
  await c.subscribe('transcribe:s');
  await c.receiveUntil('audio_config');
  c.send({ action: 'audio_start', topic: 'transcribe:s', payload: { sample_rate: 24000, channels: 1, language } });
  for (let index = 0; index < 5; index += 1) {
    c.send({ action: 'audio_chunk', topic: 'transcribe:s', payload: { index, data: Buffer.alloc(4800).toString('base64') } });
  }
  return { c, recording: fake.recording };
}

describe('elevenlabs transcriber', () => {
  it('sends the format and language to Scribe', async () => {
    const { c, recording } = await speakInto('vi');
    c.send({ action: 'audio_end', topic: 'transcribe:s' });
    const final = (await c.receiveUntil('transcript_final')).at(-1)!;
    await c.receiveUntil('utterance_end');

    expect((final.payload as { text: string }).text).toBe('hello world');
    expect(Object.fromEntries(recording.queries[0]!)).toEqual({
      model_id: 'scribe_v2_realtime',
      audio_format: 'pcm_24000',
      commit_strategy: 'vad',
      language_code: 'vi',
    });
    expect(recording.audioBytes).toBe(5 * 4800);
    expect(recording.commits).toBe(1);
  });

  it('reports speech at the first partial of an utterance', async () => {
    const { c } = await speakInto();
    const started = (await c.receiveUntil('speech_started')).at(-1)!;
    expect(started.payload).toEqual({ at_ms: 0 });
    await c.receiveUntil('transcript_partial');
  });

  it('refuses an unsupported format', () => {
    expect(() => realtimeUrl({ encoding: 'pcm16', sample_rate: 11025, channels: 1 })).toThrow(ProviderUnavailableError);
  });
});
