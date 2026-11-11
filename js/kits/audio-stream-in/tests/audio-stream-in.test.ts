import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { describe, expect, it, vi } from 'vitest';

import type { TranscriberFrame, TranscriberStream, TranscriberTopicOptions } from '..';
import { ProviderUnavailableError, TranscriptTopic, transcriberTopic } from '..';

/** Records what it is sent; on finish, says one final with every chunk's text. */
class FakeStream implements TranscriberStream {
  sent: string[] = [];
  keepalives = 0;
  closed = false;
  private queue: (TranscriberFrame | null)[] = [];
  private wake?: () => void;

  async send(audio: Buffer) {
    this.sent.push(audio.toString());
  }
  async keepalive() {
    this.keepalives += 1;
  }
  async finish() {
    this.put({ type: 'speech_started', atMs: 0 });
    this.put({ type: 'final', text: this.sent.join(' '), startMs: 0, endMs: 100 });
    this.put({ type: 'utterance_end' });
    this.put(null);
  }
  async close() {
    this.closed = true;
    this.put(null);
  }
  async *frames() {
    while (true) {
      while (this.queue.length === 0) await new Promise<void>((r) => (this.wake = r));
      const frame = this.queue.shift()!;
      if (frame === null) return;
      yield frame;
    }
  }
  private put(frame: TranscriberFrame | null) {
    this.queue.push(frame);
    this.wake?.();
  }
}

const SESSION = 'transcribe:s1';
const quietLogger = { error() {}, warn() {} };

function setup(options: Partial<TranscriberTopicOptions> = {}) {
  const streams: FakeStream[] = [];
  const topic = transcriberTopic({
    openStream: async () => {
      const stream = new FakeStream();
      streams.push(stream);
      return stream;
    },
    ...options,
  });
  const voice = consumer('/ws', {
    name: 'voice',
    authenticate: ({ url }) => url.searchParams.get('as') ?? '',
    topics: [topic, TranscriptTopic],
  });
  const chanx = createChanx({ consumers: [voice], logger: quietLogger });
  topic.attach(chanx);
  const open = async (user = 'ann') => {
    const client = connect(chanx, `/ws?as=${user}`);
    await client.receiveUntil('authentication');
    return client;
  };
  const join = async (user = 'ann') => {
    const client = await open(user);
    await client.subscribe(SESSION);
    await client.receiveUntil('audio_config');
    return client;
  };
  return { streams, open, join };
}

type Client = Awaited<ReturnType<ReturnType<typeof setup>['join']>>;

const start = (c: Client, sampleRate = 24000) =>
  c.send({ action: 'audio_start', topic: SESSION, payload: { encoding: 'pcm16', sample_rate: sampleRate, channels: 1 } });
const chunk = (c: Client, index: number, data: string) =>
  c.send({ action: 'audio_chunk', topic: SESSION, payload: { index, data: Buffer.from(data).toString('base64') } });
const endAudio = (c: Client) => c.send({ action: 'audio_end', topic: SESSION });
const lastOf = async (c: Client, action: string) => (await c.receiveUntil(action)).at(-1)!;
const errorOf = async (c: Client) => (await lastOf(c, 'transcriber_error')).payload as { code: string; message: string };

describe('transcriber', () => {
  it('sends the format on subscribe', async () => {
    const { open } = setup();
    const c = await open();
    await c.subscribe(SESSION);
    expect((await lastOf(c, 'audio_config')).payload).toEqual({
      encoding: 'pcm16',
      sample_rate: 24000,
      channels: 1,
      chunk_ms: 100,
    });
  });

  it('turns speech into an utterance', async () => {
    const { join } = setup();
    const c = await join();
    start(c);
    chunk(c, 0, 'hello');
    chunk(c, 1, 'world');
    endAudio(c);
    const final = await lastOf(c, 'transcript_final');
    const end = await lastOf(c, 'utterance_end');
    expect((final.payload as { text: string }).text).toBe('hello world');
    expect((end.payload as { utterance_id: string }).utterance_id).toBe(
      (final.payload as { utterance_id: string }).utterance_id,
    );
  });

  it('sends watchers the transcript', async () => {
    const { open, join } = setup();
    const watcher = await open('bob');
    await watcher.subscribe('transcript:s1');
    const speaker = await join();
    start(speaker);
    chunk(speaker, 0, 'hi');
    endAudio(speaker);
    expect(((await lastOf(watcher, 'transcript_final')).payload as { text: string }).text).toBe('hi');
  });

  it('refuses a format other than the configured one', async () => {
    const { join } = setup();
    const c = await join();
    start(c, 16000);
    expect((await errorOf(c)).code).toBe('bad_format');
  });

  it('refuses audio before audio_start', async () => {
    const { join } = setup();
    const c = await join();
    chunk(c, 0, 'early');
    expect((await errorOf(c)).code).toBe('not_started');
  });

  it('reports lost chunks', async () => {
    const { join } = setup({ reorderWindow: 1 });
    const c = await join();
    start(c);
    chunk(c, 0, 'a');
    chunk(c, 2, 'c');
    chunk(c, 3, 'd');
    const error = await errorOf(c);
    expect(error.code).toBe('audio_gap');
    expect(error.message).toContain('1 to 1');
  });

  it('limits streams per user', async () => {
    const { streams, join } = setup({ maxSessionsPerUser: 1 });
    const first = await join();
    const second = await join();
    start(first);
    await vi.waitFor(() => expect(streams).toHaveLength(1));
    start(second);
    expect((await errorOf(second)).code).toBe('limit_reached');
  });

  it('closes a silent stream', async () => {
    const { streams, join } = setup({ idleTimeoutSeconds: 0.05, keepaliveSeconds: null });
    const c = await join();
    start(c);
    expect((await errorOf(c)).message).toContain('No audio');
    await vi.waitFor(() => expect(streams[0]!.closed).toBe(true));
  });

  it('keeps the provider alive while quiet', async () => {
    const { streams, join } = setup({ keepaliveSeconds: 0.02, idleTimeoutSeconds: null });
    const c = await join();
    start(c);
    await vi.waitFor(() => expect(streams[0]?.keepalives).toBeGreaterThanOrEqual(2));
  });

  it('bounds a stream in time', async () => {
    const { streams, join } = setup({ maxSessionSeconds: 0.05, idleTimeoutSeconds: null });
    const c = await join();
    start(c);
    expect((await errorOf(c)).message).toContain('limited to');
    await vi.waitFor(() => expect(streams[0]!.closed).toBe(true));
  });

  it('replaces the stream on a new audio_start', async () => {
    const { streams, join } = setup();
    const c = await join();
    start(c);
    await vi.waitFor(() => expect(streams).toHaveLength(1));
    start(c);
    await vi.waitFor(() => expect(streams).toHaveLength(2));
    await vi.waitFor(() => expect(streams[0]!.closed).toBe(true));
  });

  it('closes the provider stream on leaving', async () => {
    const { streams, join } = setup();
    const c = await join();
    start(c);
    await vi.waitFor(() => expect(streams).toHaveLength(1));
    await c.unsubscribe(SESSION);
    await vi.waitFor(() => expect(streams[0]!.closed).toBe(true));
  });

  it('reports a provider that refuses, without holding a stream slot', async () => {
    const { join } = setup({
      openStream: () => Promise.reject(new ProviderUnavailableError('no key')),
      maxSessionsPerUser: 1,
    });
    const c = await join();
    start(c);
    expect((await errorOf(c)).code).toBe('provider_unavailable');
    start(c);
    expect((await errorOf(c)).code).toBe('provider_unavailable');
  });

  it('says on subscribe when the provider is not configured', async () => {
    const { open } = setup({ providerAvailable: () => false });
    const c = await open();
    await c.subscribe(SESSION);
    expect((await errorOf(c)).code).toBe('provider_unavailable');
  });

  it('lets final hooks see the text', async () => {
    const heard: string[] = [];
    const { join } = setup({ onFinal: (scope, final) => void heard.push(`${scope.session}:${final.text}`) });
    const c = await join();
    start(c);
    chunk(c, 0, 'hey');
    endAudio(c);
    await vi.waitFor(() => expect(heard).toEqual(['s1:hey']));
  });
});
