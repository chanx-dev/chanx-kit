import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { describe, expect, it, vi } from 'vitest';

import type { Synthesize, SynthesizerTopicOptions } from '..';
import { SynthesizerUnavailableError, heardText, synthesizerTopic } from '..';

/** `n` bytes in two pieces, the first ending mid-sample. */
const tone =
  (n: number): Synthesize =>
  async function* () {
    const audio = new Uint8Array(n);
    yield audio.subarray(0, n / 2 + 1);
    yield audio.subarray(n / 2 + 1);
  };

/** One piece, then waits for `release` or an abort. */
function slow() {
  let release!: () => void;
  const released = new Promise<void>((r) => (release = r));
  const synthesize: Synthesize = async function* (_text, _voice, _format, signal) {
    yield new Uint8Array(4800);
    await Promise.race([released, new Promise((r) => signal.addEventListener('abort', r))]);
  };
  return { synthesize, release };
}

let sessions = 0;

function setup(options: SynthesizerTopicOptions) {
  const topic = synthesizerTopic(options);
  const voice = consumer('/ws', { name: 'voice', topics: [topic] });
  const chanx = createChanx({ consumers: [voice], logger: { error() {}, warn() {} } });
  const session = `s${(sessions += 1)}`;
  const name = `speak:${session}`;
  const listen = async () => {
    const client = connect(chanx, '/ws');
    await client.subscribe(name);
    return client;
  };
  return { topic, chanx, session, name, listen };
}

type Client = Awaited<ReturnType<ReturnType<typeof setup>['listen']>>;
const lastOf = async (c: Client, action: string) => (await c.receiveUntil(action)).at(-1)!;
const payload = <T>(frame: unknown) => (frame as { payload: T }).payload;
const say = (c: Client, name: string, text: string) => c.send({ action: 'speak', topic: name, payload: { text } });

describe('synthesizer', () => {
  it('turns text into one utterance of PCM audio', async () => {
    const { name, listen } = setup({ synthesize: tone(9600) });
    const c = await listen();
    say(c, name, 'hello');
    const frames = await c.receiveUntil('audio_end');
    const start = payload<{ text: string; sample_rate: number }>(frames.find((f) => f.action === 'audio_start')!);
    expect(start).toMatchObject({ text: 'hello', sample_rate: 24000 });
    expect(payload<{ duration_ms: number }>(frames.at(-1)!).duration_ms).toBe(200);
  });

  it('cuts audio into bounded whole frames', async () => {
    const { name, listen } = setup({ synthesize: tone(24001), maxChunkMs: 100 });
    const c = await listen();
    say(c, name, 'hello');
    const chunks = (await c.receiveUntil('audio_end'))
      .filter((f) => f.action === 'audio_chunk')
      .map((f) => payload<{ data: unknown }>(f).data);
    expect(chunks.every((data) => typeof data === 'string')).toBe(true);
    const sizes = chunks.map((data) => Buffer.from(data as string, 'base64').length);
    expect(sizes.every((n) => n % 2 === 0 && n <= 4800)).toBe(true);
    expect(sizes.reduce((a, b) => a + b, 0)).toBe(24000);
  });

  it('plays utterances in the order they were queued', async () => {
    const { topic, chanx, session, listen } = setup({ synthesize: tone(480) });
    const c = await listen();
    for (const text of ['one', 'two', 'three']) await topic.speak(chanx, session, text);
    const texts: string[] = [];
    while (texts.length < 3) texts.push(payload<{ text: string }>(await lastOf(c, 'audio_start')).text);
    expect(texts).toEqual(['one', 'two', 'three']);
  });

  it('reaches every listener', async () => {
    const { name, listen } = setup({ synthesize: tone(480) });
    const asker = await listen();
    const other = await listen();
    say(asker, name, 'hi all');
    expect(payload<{ text: string }>(await lastOf(other, 'audio_start')).text).toBe('hi all');
  });

  it('replays a listener joining mid-utterance from its start', async () => {
    const { synthesize, release } = slow();
    const { topic, session, name, listen } = setup({ synthesize });
    const first = await listen();
    say(first, name, 'long one');
    await first.receiveUntil('audio_chunk');
    const late = connectAndCollect(await listen());
    await vi.waitFor(() => expect(late.actions()).toEqual(['subscribed', 'audio_start', 'audio_chunk']));
    release();
    await topic.waitUntilSpoken(session);
  });

  it('clear stops the utterance and drops the queue', async () => {
    const { synthesize } = slow();
    const { topic, chanx, session, name, listen } = setup({ synthesize });
    const c = await listen();
    const id = await topic.speak(chanx, session, 'first');
    await topic.speak(chanx, session, 'second');
    await c.receiveUntil('audio_chunk');
    c.send({ action: 'speak_clear', topic: name });
    expect(payload<{ utterance_id: string }>(await lastOf(c, 'clear')).utterance_id).toBe(id);
    expect(topic.isSpeaking(session)).toBe(false);
    expect(await c.receiveNothing({ timeout: 50 })).toBe(true);
  });

  it('refuses empty text', async () => {
    const { name, listen } = setup({ synthesize: tone(480) });
    const c = await listen();
    say(c, name, '  ');
    expect(payload<{ code: string }>(await lastOf(c, 'synthesizer_error')).code).toBe('bad_request');
  });

  it('refuses text over the limit', async () => {
    const { name, listen } = setup({ synthesize: tone(480), maxTextChars: 3 });
    const c = await listen();
    say(c, name, 'four');
    expect(payload<{ code: string }>(await lastOf(c, 'synthesizer_error')).code).toBe('bad_request');
  });

  it('reports a refusing provider and still ends the utterance', async () => {
    const { name, listen } = setup({
      synthesize: async function* () {
        yield* [];
        throw new SynthesizerUnavailableError('no key');
      },
    });
    const c = await listen();
    say(c, name, 'hello');
    expect(payload<{ code: string }>(await lastOf(c, 'synthesizer_error')).code).toBe('provider_unavailable');
    await c.receiveUntil('audio_end');
  });

  it('answers the asker when the provider is not configured', async () => {
    const { name, listen } = setup({ synthesize: tone(480), providerAvailable: () => false });
    const c = await listen();
    say(c, name, 'hello');
    expect(payload<{ code: string }>(await lastOf(c, 'synthesizer_error')).code).toBe('provider_unavailable');
  });

  it('records playback marks', async () => {
    const { topic, session, name, listen } = setup({ synthesize: tone(480) });
    const c = await listen();
    say(c, name, 'hello');
    const id = payload<{ utterance_id: string }>(await lastOf(c, 'audio_start')).utterance_id;
    await c.receiveUntil('audio_end');
    c.send({ action: 'playback_mark', topic: name, payload: { utterance_id: id, played_ms: 7 } });
    await vi.waitFor(async () =>
      expect(await topic.playbackStore.get(session, id)).toMatchObject({ playedMs: 7, finished: true }),
    );
  });

  it.each([
    [0, ''],
    [500, 'The quick brown'],
    [1000, 'The quick brown fox jumps over'],
    [5000, 'The quick brown fox jumps over'],
  ])('heard text after %i ms is cut at a word', (played, heard) => {
    expect(heardText('The quick brown fox jumps over', 1000, played)).toBe(heard);
  });

  it('heard text of an unmeasured utterance is empty', () => {
    expect(heardText('anything', 0, 100)).toBe('');
  });
});

/** Keeps every frame a client receives, for asserting on order. */
function connectAndCollect(client: Client) {
  const seen: string[] = ['subscribed'];
  void (async () => {
    for (;;) {
      try {
        seen.push((await client.receive({ timeout: 2000 })).action as string);
      } catch {
        return;
      }
    }
  })();
  return { actions: () => seen };
}
