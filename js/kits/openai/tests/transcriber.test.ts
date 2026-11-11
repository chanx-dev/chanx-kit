import type { AddressInfo } from 'node:net';

import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import type { RawData } from 'ws';
import { WebSocketServer } from 'ws';
import { afterEach, describe, expect, it } from 'vitest';

import { openAITranscriberTopic } from '../transcriber';

const KEY = 'test-key';
const closers: (() => void)[] = [];
afterEach(() => closers.splice(0).forEach((close) => close()));

/** Hears "hello" after 400 ms of audio, then " world" and the transcript on commit. */
async function fakeRealtime() {
  const recording = { sessions: [] as Record<string, any>[], audioBytes: 0, commits: 0 };
  const server = new WebSocketServer({
    port: 0,
    verifyClient: ({ req }, done) => done(req.headers.authorization === `Bearer ${KEY}`, 401),
  });
  server.on('connection', (socket) => {
    const send = (event: unknown) => socket.send(JSON.stringify(event));
    let received = 0;
    let spoke = false;
    let item = 0;
    socket.on('message', (data: RawData) => {
      const event = JSON.parse(data.toString());
      const id = `i${item}`;
      if (event.type === 'session.update') recording.sessions.push(event.session);
      if (event.type === 'input_audio_buffer.append') {
        const size = Buffer.from(event.audio, 'base64').length;
        received += size;
        recording.audioBytes += size;
        if (!spoke && received >= (24000 * 2 * 4) / 10) {
          spoke = true;
          send({ type: 'conversation.item.input_audio_transcription.delta', item_id: id, delta: 'hello' });
        }
      }
      if (event.type === 'input_audio_buffer.commit') {
        recording.commits += 1;
        if (!spoke) {
          send({ type: 'error', error: { code: 'input_audio_buffer_commit_empty', message: 'buffer too small' } });
          return;
        }
        send({ type: 'conversation.item.input_audio_transcription.delta', item_id: id, delta: ' world' });
        send({ type: 'conversation.item.input_audio_transcription.completed', item_id: id, transcript: 'hello world' });
        item += 1;
      }
    });
  });
  await new Promise((r) => server.once('listening', r));
  closers.push(() => server.close());
  return { url: `ws://127.0.0.1:${(server.address() as AddressInfo).port}`, recording };
}

async function join(url: string) {
  const topic = openAITranscriberTopic({ apiKey: KEY, url });
  const chanx = createChanx({ consumers: [consumer('/ws', { name: 'voice', topics: [topic] })] });
  const c = connect(chanx, '/ws');
  await c.subscribe('transcribe:s');
  await c.receiveUntil('audio_config');
  return c;
}

function utterance(c: Awaited<ReturnType<typeof join>>, chunks: number, language?: string) {
  c.send({ action: 'audio_start', topic: 'transcribe:s', payload: { sample_rate: 24000, channels: 1, language } });
  for (let index = 0; index < chunks; index += 1) {
    c.send({ action: 'audio_chunk', topic: 'transcribe:s', payload: { index, data: Buffer.alloc(4800).toString('base64') } });
  }
  c.send({ action: 'audio_end', topic: 'transcribe:s' });
}

describe('openai transcriber', () => {
  it('configures the session for transcription', async () => {
    const { url, recording } = await fakeRealtime();
    const c = await join(url);
    utterance(c, 5, 'en');
    const frames = await c.receiveUntil('transcript_final');
    await c.receiveUntil('utterance_end');

    const partials = frames.filter((f) => f.action === 'transcript_partial').map((f) => (f.payload as { text: string }).text);
    expect(partials).toEqual(['hello', 'hello world']);
    const session = recording.sessions[0]!;
    expect(session.type).toBe('transcription');
    expect(session.audio.input).toEqual({
      format: { type: 'audio/pcm', rate: 24000 },
      transcription: { model: 'gpt-live-transcribe', language: 'en' },
      turn_detection: null,
    });
    expect(recording.commits).toBe(1);
  });

  it('does not treat ending without speech as an error', async () => {
    const { url } = await fakeRealtime();
    const c = await join(url);
    utterance(c, 1);
    utterance(c, 5);
    const frames = await c.receiveUntil('transcript_final');
    expect(frames.filter((f) => f.action === 'transcriber_error')).toEqual([]);
  });
});
