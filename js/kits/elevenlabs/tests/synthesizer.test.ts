import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { SynthesizerUnavailableError } from '../../audio-stream-out';
import { elevenLabsSynthesizerTopic, refusal, streamUrl } from '../synthesizer';
import type { ElevenLabsSynthesizerOptions } from '../synthesizer';
import { KEY, fakeElevenLabs, streamInput } from './fake';

const closers: (() => void)[] = [];
afterEach(() => closers.splice(0).forEach((close) => close()));

async function say(options: Omit<ElevenLabsSynthesizerOptions, 'url'>, payload: { text: string; voice?: string }) {
  const fake = await fakeElevenLabs(streamInput);
  closers.push(fake.close);
  const url = `${fake.url}/v1/text-to-speech/{voice_id}/stream-input`;
  const topic = elevenLabsSynthesizerTopic({ ...options, url });
  const chanx = createChanx({ consumers: [consumer('/ws', { name: 'voice', topics: [topic] })] });
  const c = connect(chanx, '/ws');
  await c.subscribe('speak:s');
  c.send({ action: 'speak', topic: 'speak:s', payload });
  return { c, recording: fake.recording };
}

describe('elevenlabs synthesizer', () => {
  it('sends the voice, format and text to ElevenLabs', async () => {
    const { c, recording } = await say({ apiKey: KEY }, { text: 'Xin chao', voice: 'voice-9' });
    const end = (await c.receiveUntil('audio_end')).at(-1)!;

    expect((end.payload as { duration_ms: number }).duration_ms).toBe(160);
    expect(recording.paths).toEqual(['/v1/text-to-speech/voice-9/stream-input']);
    expect(Object.fromEntries(recording.queries[0]!)).toEqual({
      model_id: 'eleven_flash_v2_5',
      output_format: 'pcm_24000',
    });
    expect(recording.messages.map((m) => m.text)).toEqual([' ', 'Xin chao ', '']);
    expect(recording.messages[0]!.voice_settings).toEqual({ stability: 0.5, similarity_boost: 0.75 });
  });

  it('reports a wrong key', async () => {
    const { c } = await say({ apiKey: 'nope' }, { text: 'hi' });
    const error = (await c.receiveUntil('synthesizer_error')).at(-1)!.payload as { code: string; message: string };
    expect(error.code).toBe('provider_unavailable');
    expect(error.message).toContain('401');
  });

  it('refuses an unsupported format up front', () => {
    expect(() => streamUrl('v', { encoding: 'pcm16', sample_rate: 11025, channels: 1 })).toThrow(SynthesizerUnavailableError);
    expect(() => streamUrl('v', { encoding: 'pcm16', sample_rate: 24000, channels: 2 })).toThrow(SynthesizerUnavailableError);
  });

  it('is unavailable without a key', async () => {
    vi.stubEnv('ELEVENLABS_API_KEY', '');
    const topic = elevenLabsSynthesizerTopic();
    const chanx = createChanx({ consumers: [consumer('/ws', { name: 'voice', topics: [topic] })] });
    const c = connect(chanx, '/ws');
    await c.subscribe('speak:s');
    c.send({ action: 'speak', topic: 'speak:s', payload: { text: 'hi' } });
    const error = (await c.receiveUntil('synthesizer_error')).at(-1)!.payload as { code: string };
    expect(error.code).toBe('provider_unavailable');
    vi.unstubAllEnvs();
  });

  it('says what to change on a plan refusal', () => {
    expect(refusal('payment_required', 'v1')).toBe(
      'ElevenLabs: payment_required (voice v1 is not available on this plan; use a premade voice)',
    );
    expect(refusal('something_new', 'v1')).toBe('ElevenLabs: something_new');
  });
});
