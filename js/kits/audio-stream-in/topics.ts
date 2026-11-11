import { randomUUID } from 'node:crypto';

import type { Chanx, Connection, Message } from '@chanx-js/server';
import { topic } from '@chanx-js/server';

import { ChunkSequencer, MediaIntake } from '../media-stream-in';
import type { AudioConfig, AudioStart, SpeechStarted, TranscriberErrorCode, TranscriptFinal, UtteranceEnd } from './messages';
import {
  AudioChunkMessage,
  AudioConfig as AudioConfigSchema,
  AudioConfigMessage,
  AudioConfigRequestMessage,
  AudioEndMessage,
  AudioStartMessage,
  SpeechStartedMessage,
  TRANSCRIPT_EVENTS,
  TranscriberErrorMessage,
  TranscriptFinalMessage,
  TranscriptPartialMessage,
  UtteranceEndMessage,
} from './messages';
import type { TranscriberFrame, TranscriberStream } from './provider';
import { ProviderUnavailableError } from './provider';

/** `transcript:<session>`: a session's transcripts, read-only. */
export const TranscriptTopic = topic('transcript:{session}', {
  name: 'TranscriptTopic',
  description: "Watch a session's transcripts read-only.",
}).passthrough(...TRANSCRIPT_EVENTS);

/** The session a hook is called for. */
export interface Scope {
  session: string;
  conn: Connection<string, unknown>;
}

export interface TranscriberTopicOptions {
  openStream: (audio: AudioStart) => Promise<TranscriberStream>;
  /** False when the provider cannot work, e.g. without a key. */
  providerAvailable?: () => boolean;
  audioConfig?: Partial<AudioConfig>;
  /** Who stream limits count against; undefined counts per connection. */
  userId?: (conn: Connection<string, unknown>) => string | undefined;
  // Providers bill per second, so streams are bounded; null turns a limit off.
  maxSessionSeconds?: number | null;
  idleTimeoutSeconds?: number | null;
  maxSessionsPerUser?: number | null;
  keepaliveSeconds?: number | null;
  finishTimeoutSeconds?: number;
  queueChunks?: number;
  reorderWindow?: number;
  // Hooks run after the event is sent.
  onSpeechStarted?: (scope: Scope, event: SpeechStarted) => Promise<void> | void;
  onFinal?: (scope: Scope, transcript: TranscriptFinal) => Promise<void> | void;
  onUtteranceEnd?: (scope: Scope, event: UtteranceEnd) => Promise<void> | void;
  name?: string;
}

/** A string user, or one with `id`; undefined otherwise. */
export function userIdOf(conn: Connection<string, unknown>): string | undefined {
  const user = conn.user as string | { id?: unknown } | null | undefined;
  if (typeof user === 'string' && user) return user;
  if (typeof user === 'object' && user?.id != null) return String(user.id);
  return undefined;
}

/** One provider stream; holds chunks that arrive while it opens. */
class Session {
  stream?: TranscriberStream;
  intake?: MediaIntake;
  held: Buffer[] = [];
  endRequested = false;
  closed = false;
  utteranceId?: string;
  listening?: Promise<void>;
  timer?: NodeJS.Timeout;
  counted?: string;

  constructor(readonly sequencer: ChunkSequencer) {}

  feed(chunk: Buffer) {
    if (this.intake) this.intake.feed(chunk);
    else this.held.push(chunk);
  }
}

const value = <T>(option: T | null | undefined, fallback: T) => (option === undefined ? fallback : option);

/** `transcribe:<session>`: audio in, transcripts out. Call `attach(chanx)` for transcript watchers. */
export function transcriberTopic(options: TranscriberTopicOptions) {
  const audioConfig = AudioConfigSchema.parse(options.audioConfig ?? {});
  const maxSession = value(options.maxSessionSeconds, 300);
  const idleTimeout = value(options.idleTimeoutSeconds, 15);
  const maxPerUser = value(options.maxSessionsPerUser, 2);
  const keepalive = value(options.keepaliveSeconds, 4);
  const finishTimeout = options.finishTimeoutSeconds ?? 10;
  const available = () => options.providerAvailable?.() ?? true;
  // Per user, this process only: a stream lives where its socket is.
  const open = new Map<string, number>();
  let app: Chanx | undefined;

  const errorMessage = (code: TranscriberErrorCode, message: string) =>
    TranscriberErrorMessage.create({ code, message });

  const built = topic('transcribe:{session}', {
    name: options.name ?? 'TranscriberTopic',
    description: 'Audio in, transcripts out.',
    state: () => ({ session: undefined as Session | undefined }),
    onSubscribe: async (ctx) => {
      await ctx.push(AudioConfigMessage.create(audioConfig));
      if (!available()) {
        await ctx.push(errorMessage('provider_unavailable', 'The transcription provider is not configured.'));
      }
    },
    onUnsubscribe: async (ctx) => {
      const session = ctx.state.session;
      ctx.state.session = undefined;
      if (session) await close(session, false);
    },
  })
    .on(
      AudioConfigRequestMessage,
      {
        output: AudioConfigMessage,
        summary: 'Ask for the capture format',
        description: 'Return the format to capture audio in, without resubscribing.',
      },
      () => AudioConfigMessage.create(audioConfig),
    )
    .on(
      AudioStartMessage,
      {
        output: [...TRANSCRIPT_EVENTS, TranscriberErrorMessage],
        summary: 'Start streaming audio',
        description: 'Open a provider stream for audio in the negotiated format.',
      },
      async (m, ctx) => {
        const scope: Scope = { session: ctx.params.session, conn: ctx.conn as Connection<string, unknown> };
        const send = (message: Message) => ctx.push(message as never).catch(() => {});
        const publish = async (message: Message) => {
          await send(message);
          await app?.publish(TranscriptTopic, TranscriptTopic.with({ session: scope.session }), message as never);
        };

        const previous = ctx.state.session;
        const session = new Session(new ChunkSequencer(options.reorderWindow ?? 8));
        ctx.state.session = session;
        if (previous) await close(previous, false);

        const abandon = async (code: TranscriberErrorCode, text: string) => {
          session.closed = true;
          if (ctx.state.session === session) ctx.state.session = undefined;
          await send(errorMessage(code, text));
        };
        const audio = m.payload;
        if (
          audio.encoding !== audioConfig.encoding ||
          audio.sample_rate !== audioConfig.sample_rate ||
          audio.channels !== audioConfig.channels
        ) {
          return abandon(
            'bad_format',
            `Expected ${audioConfig.encoding} at ${audioConfig.sample_rate} Hz, ${audioConfig.channels} channel(s).`,
          );
        }
        if (!available()) return abandon('provider_unavailable', 'The transcription provider is not configured.');
        const identity = (options.userId ?? userIdOf)(scope.conn) ?? `connection:${scope.conn.channelName}`;
        if (maxPerUser !== null && (open.get(identity) ?? 0) >= maxPerUser) {
          return abandon('limit_reached', `At most ${maxPerUser} streams at once.`);
        }
        open.set(identity, (open.get(identity) ?? 0) + 1);
        session.counted = identity;

        let stream: TranscriberStream;
        try {
          stream = await options.openStream(audio);
        } catch (error) {
          if (!(error instanceof ProviderUnavailableError)) console.error('opening the transcriber stream failed', error);
          release(session);
          return abandon('provider_unavailable', String((error as Error)?.message ?? error));
        }
        if (session.closed) {
          // Replaced or left while the provider was connecting.
          await stream.close();
          return;
        }

        session.stream = stream;
        session.intake = new MediaIntake((chunk) => stream.send(chunk), {
          onEnd: () => stream.finish(),
          onQuiet: async (quiet) => {
            if (quiet === 'keepalive') return stream.keepalive();
            await send(errorMessage('limit_reached', 'No audio arrived for too long.'));
            await close(session, false);
          },
          maxChunks: options.queueChunks ?? 50,
          keepaliveSeconds: keepalive ?? undefined,
          idleSeconds: idleTimeout ?? undefined,
        });
        session.listening = listen(session, scope, stream, send, publish);
        session.intake.start();
        for (const chunk of session.held) session.intake.feed(chunk);
        session.held = [];
        if (maxSession !== null) {
          session.timer = setTimeout(async () => {
            if (session.closed) return;
            await send(errorMessage('limit_reached', `Streams are limited to ${maxSession} seconds.`));
            await close(session, false);
          }, maxSession * 1000);
        }
        if (session.endRequested) end(session);
      },
    )
    .on(
      AudioChunkMessage,
      {
        output: TranscriberErrorMessage,
        summary: 'Send audio',
        description: 'One chunk of the stream, numbered from 0 per audio_start.',
      },
      async (m, ctx) => {
        const session = ctx.state.session;
        if (!session || session.closed || session.endRequested) {
          await ctx.push(errorMessage('not_started', 'Send audio_start before audio.'));
          return;
        }
        const [ready, gaps] = session.sequencer.push(m.payload.index, m.payload.data);
        for (const chunk of ready) session.feed(chunk);
        for (const gap of gaps) {
          await ctx.push(errorMessage('audio_gap', `Chunks ${gap.start} to ${gap.end - 1} were lost.`));
        }
      },
    )
    .on(
      AudioEndMessage,
      {
        output: TranscriberErrorMessage,
        summary: 'End the audio',
        description: 'Flush the provider, deliver its last transcripts, then close.',
      },
      async (_m, ctx) => {
        // Without a payload it validates sooner and can overtake the frames before it.
        await new Promise((resolve) => setImmediate(resolve));
        const session = ctx.state.session;
        if (!session || session.closed || session.endRequested) return;
        session.endRequested = true;
        const [ready, gaps] = session.sequencer.flush();
        for (const chunk of ready) session.feed(chunk);
        if (session.intake) end(session);
        for (const gap of gaps) {
          await ctx.push(errorMessage('audio_gap', `Chunks ${gap.start} to ${gap.end - 1} were lost.`));
        }
      },
    );

  function release(session: Session) {
    const identity = session.counted;
    session.counted = undefined;
    if (identity === undefined) return;
    const remaining = (open.get(identity) ?? 0) - 1;
    if (remaining > 0) open.set(identity, remaining);
    else open.delete(identity);
  }

  function end(session: Session) {
    session.intake!.end();
    void (async () => {
      let timer: NodeJS.Timeout | undefined;
      const finished = await Promise.race([
        session.listening!.then(() => true),
        new Promise<false>((resolve) => (timer = setTimeout(() => resolve(false), finishTimeout * 1000))),
      ]);
      clearTimeout(timer);
      await close(session, finished);
    })();
  }

  async function close(session: Session, graceful: boolean) {
    if (session.closed && !session.stream) return;
    session.closed = true;
    const stream = session.stream;
    session.stream = undefined;
    clearTimeout(session.timer);
    await session.intake?.stop();
    if (!graceful) await stream?.close().catch(() => {});
    release(session);
  }

  async function listen(
    session: Session,
    scope: Scope,
    stream: TranscriberStream,
    send: (m: Message) => Promise<void>,
    publish: (m: Message) => Promise<void>,
  ) {
    const utterance = () => (session.utteranceId ??= randomUUID().replaceAll('-', ''));
    const utteranceEnd = async (utteranceId: string) => {
      const event = { utterance_id: utteranceId };
      await publish(UtteranceEndMessage.create(event));
      await options.onUtteranceEnd?.(scope, event);
    };
    try {
      for await (const frame of stream.frames() as AsyncIterable<TranscriberFrame>) {
        if (session.closed) return;
        switch (frame.type) {
          case 'speech_started': {
            const event = { at_ms: frame.atMs };
            await publish(SpeechStartedMessage.create(event));
            await options.onSpeechStarted?.(scope, event);
            break;
          }
          case 'partial':
            await publish(
              TranscriptPartialMessage.create({
                utterance_id: utterance(),
                text: frame.text,
                start_ms: frame.startMs,
                end_ms: frame.endMs,
              }),
            );
            break;
          case 'final': {
            const final = {
              utterance_id: utterance(),
              text: frame.text,
              start_ms: frame.startMs,
              end_ms: frame.endMs,
              confidence: frame.confidence ?? null,
            };
            await publish(TranscriptFinalMessage.create(final));
            await options.onFinal?.(scope, final);
            break;
          }
          case 'utterance_end':
            if (session.utteranceId) {
              const id = session.utteranceId;
              session.utteranceId = undefined;
              await utteranceEnd(id);
            }
            break;
        }
      }
    } catch (error) {
      if (session.closed) return;
      console.error('transcriber stream failed', error);
      await send(errorMessage('provider_unavailable', String((error as Error)?.message ?? error)));
      await close(session, false);
      return;
    }
    // The provider closed the stream: after audio_end, or on its own.
    if (session.utteranceId) {
      const id = session.utteranceId;
      session.utteranceId = undefined;
      await utteranceEnd(id);
    }
    await close(session, true);
  }

  return Object.assign(built, {
    /** The app, so transcripts also reach `transcript:<session>` watchers. */
    attach(chanx: Chanx) {
      app = chanx;
    },
  });
}
