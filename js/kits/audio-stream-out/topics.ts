import { randomUUID } from 'node:crypto';

import type { Chanx, Message, PublishOptions } from '@chanx-js/server';
import { topic } from '@chanx-js/server';

import type { ReplayStore } from '../media-stream-out';
import { InMemoryReplayStore, SerialJobs } from '../media-stream-out';
import type { SpeechFormat, SynthesizerErrorCode } from './messages';
import {
  PlaybackMarkMessage,
  SPEECH_EVENTS,
  SpeakClearMessage,
  SpeakMessage,
  SpeechAudioChunkMessage,
  SpeechAudioEndMessage,
  SpeechAudioStartMessage,
  SpeechClearMessage,
  SpeechFormat as SpeechFormatSchema,
  SynthesizerErrorMessage,
} from './messages';
import type { PlaybackStore } from './playback';
import { InMemoryPlaybackStore } from './playback';

/** Thrown by a provider that refuses, such as without a key. */
export class SynthesizerUnavailableError extends Error {}

/** Yields PCM16 audio for `text`. */
export type Synthesize = (
  text: string,
  voice: string | undefined,
  format: SpeechFormat,
  signal: AbortSignal,
) => AsyncIterable<Uint8Array>;

export interface SynthesizerTopicOptions {
  synthesize: Synthesize;
  /** False when the provider cannot work, e.g. without a key. */
  providerAvailable?: () => boolean;
  outputFormat?: Partial<SpeechFormat>;
  defaultVoice?: string;
  /** Longer text is refused rather than billed. */
  maxTextChars?: number;
  maxChunkMs?: number;
  replayStore?: ReplayStore<Message>;
  playbackStore?: PlaybackStore;
  onPlaybackMark?: (session: string, utteranceId: string, playedMs: number) => Promise<void> | void;
  /** Group namespace; two synthesizers on one server need different names. */
  name?: string;
}

type Publish = (message: Message, options?: PublishOptions) => Promise<void>;

/** `speak:<session>`: text to speech for everyone on it. */
export function synthesizerTopic(options: SynthesizerTopicOptions) {
  const name = options.name ?? 'SynthesizerTopic';
  const format = SpeechFormatSchema.parse(options.outputFormat ?? {});
  const maxTextChars = options.maxTextChars ?? 2000;
  const maxChunkMs = options.maxChunkMs ?? 200;
  const replayStore = options.replayStore ?? new InMemoryReplayStore<Message>();
  const playbackStore = options.playbackStore ?? new InMemoryPlaybackStore();
  const available = () => options.providerAvailable?.() ?? true;
  const jobs = new SerialJobs();
  const key = (session: string) => `${name}:${session}`;

  const error = (code: SynthesizerErrorCode, message: string, utteranceId?: string | null) =>
    SynthesizerErrorMessage.create({ code, message, utterance_id: utteranceId ?? null });

  /** Records `message` for late joiners, then publishes it. */
  async function emit(publish: Publish, session: string, message: Message) {
    const seq = await replayStore.append(key(session), message);
    await publish(message, { seq });
  }

  async function produce(publish: Publish, session: string, utteranceId: string, text: string, voice: string | undefined, signal: AbortSignal) {
    const frameBytes = format.channels * 2;
    const maxBytes = Math.max(frameBytes, Math.floor((format.sample_rate * maxChunkMs) / 1000) * frameBytes);
    await replayStore.clear(key(session));
    await playbackStore.spoken(session, utteranceId, text);
    await emit(publish, session, SpeechAudioStartMessage.create({ ...format, utterance_id: utteranceId, text }));

    let index = 0;
    let total = 0;
    let held = Buffer.alloc(0); // a provider chunk can end mid-sample
    try {
      for await (const piece of options.synthesize(text, voice, format, signal)) {
        if (signal.aborted) return;
        held = Buffer.concat([held, piece]);
        const usable = held.length - (held.length % frameBytes);
        for (let at = 0; at < usable; at += maxBytes) {
          const data = held.subarray(at, Math.min(at + maxBytes, usable));
          await emit(
            publish,
            session,
            SpeechAudioChunkMessage.create({ utterance_id: utteranceId, index, data: data.toString('base64') }),
          );
          index += 1;
          total += data.length;
        }
        held = Buffer.from(held.subarray(usable));
      }
    } catch (e) {
      if (signal.aborted) return;
      if (e instanceof SynthesizerUnavailableError) {
        await publish(error('provider_unavailable', e.message, utteranceId));
      } else {
        console.error('synthesis failed', e);
        await publish(error('provider_failed', String((e as Error)?.message ?? e), utteranceId));
      }
    }
    if (signal.aborted) return;
    const durationMs = Math.floor((total * 1000) / (format.sample_rate * frameBytes));
    await playbackStore.finished(session, utteranceId, durationMs);
    await emit(publish, session, SpeechAudioEndMessage.create({ utterance_id: utteranceId, duration_ms: durationMs }));
    await replayStore.clear(key(session));
  }

  async function speak(publish: Publish, session: string, rawText: string, voice?: string | null, utteranceId?: string | null) {
    const utterance = utteranceId || randomUUID().replaceAll('-', '');
    const text = rawText.trim();
    if (!text) {
      await publish(error('bad_request', 'Nothing to say.', utterance));
      return utterance;
    }
    if ([...text].length > maxTextChars) {
      await publish(error('bad_request', `Text is limited to ${maxTextChars} characters.`, utterance));
      return utterance;
    }
    jobs.submit(key(session), (signal) =>
      produce(publish, session, utterance, text, voice ?? options.defaultVoice, signal),
    );
    return utterance;
  }

  async function clear(publish: Publish, session: string) {
    const buffered = await replayStore.replay(key(session));
    const current = (buffered.at(-1)?.[1] as { payload?: { utterance_id?: string } } | undefined)?.payload
      ?.utterance_id;
    await jobs.cancel(key(session));
    await replayStore.clear(key(session));
    await publish(SpeechClearMessage.create({ utterance_id: current ?? null }));
  }

  const built = topic('speak:{session}', {
    name,
    description: 'Text to speech for everyone on the session.',
    // Replay the utterance in flight: audio cannot be joined part-way.
    onSubscribe: async (ctx) => {
      for (const [, message] of await replayStore.replay(key(ctx.params.session))) {
        await ctx.push(message as never);
      }
    },
  })
    .on(
      SpeakMessage,
      {
        output: [SpeechAudioStartMessage, SpeechAudioChunkMessage, SpeechAudioEndMessage, SynthesizerErrorMessage],
        summary: 'Speak text',
        description: 'Queue text to be spoken to everyone on the session.',
      },
      async (m, ctx) => {
        if (!available()) {
          await ctx.push(error('provider_unavailable', 'The speech provider is not configured.', m.payload.utterance_id));
          return;
        }
        await speak((msg, o) => ctx.publish(msg, o), ctx.params.session, m.payload.text, m.payload.voice, m.payload.utterance_id);
      },
    )
    .on(
      SpeakClearMessage,
      {
        output: SpeechClearMessage,
        summary: 'Stop speaking',
        description: 'Stop the utterance playing and drop everything queued.',
      },
      async (_m, ctx) => {
        await clear((msg, o) => ctx.publish(msg, o), ctx.params.session);
      },
    )
    .on(
      PlaybackMarkMessage,
      { summary: 'Report playback', description: 'How much of an utterance the client has actually played.' },
      async (m, ctx) => {
        await playbackStore.mark(ctx.params.session, m.payload.utterance_id, m.payload.played_ms);
        await options.onPlaybackMark?.(ctx.params.session, m.payload.utterance_id, m.payload.played_ms);
      },
    )
    .passthrough(...SPEECH_EVENTS);

  const publisher =
    (chanx: Chanx, session: string): Publish =>
    (message, publishOptions) =>
      chanx.publish(built, built.with({ session }), message as never, publishOptions);

  return Object.assign(built, {
    playbackStore,
    /** Queue `text` on `session` from the server; returns its utterance id. */
    speak: (chanx: Chanx, session: string, text: string, voice?: string, utteranceId?: string) =>
      speak(publisher(chanx, session), session, text, voice, utteranceId),
    /** Stop the utterance playing and drop the queued ones. */
    clear: (chanx: Chanx, session: string) => clear(publisher(chanx, session), session),
    /** Whether `session` has speech playing or queued in this process. */
    isSpeaking: (session: string) => jobs.busy(key(session)),
    /** Until `session`'s queue is sent. For tests. */
    waitUntilSpoken: (session: string) => jobs.wait(key(session)),
  });
}
