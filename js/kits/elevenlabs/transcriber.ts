import type WebSocket from 'ws';

import type { AudioStart, TranscriberFrame, TranscriberStream, TranscriberTopicOptions } from '../audio-stream-in';
import { ProviderUnavailableError, transcriberTopic } from '../audio-stream-in';
import { Inbox, PCM_RATES, dial, keyOf, sendJson, supported } from './elevenlabs';

const ERRORS = new Set([
  'error',
  'auth_error',
  'quota_exceeded',
  'rate_limited',
  'resource_exhausted',
  'session_time_limit_exceeded',
  'chunk_size_exceeded',
  'input_error',
]);

/** One Scribe realtime session: base64 PCM up, partial and committed text down. */
export class ElevenLabsTranscriberStream implements TranscriberStream {
  private ms = 0;
  private segmentStart = 0;
  private finishing = false;
  // Scribe has no speech-started event; an utterance's first partial stands in.
  private inUtterance = false;
  private done = false;
  private readonly inbox: Inbox;

  constructor(
    private readonly socket: WebSocket,
    private readonly rate: number,
  ) {
    this.inbox = new Inbox(socket, () => this.finishing);
  }

  async send(audio: Buffer) {
    this.chunk(audio, false);
    this.ms += (audio.length / (this.rate * 2)) * 1000;
  }

  /** An empty chunk: Scribe has no keepalive message. */
  async keepalive() {
    this.chunk(Buffer.alloc(0), false);
  }

  async finish() {
    this.finishing = true;
    this.chunk(Buffer.alloc(0), true);
  }

  async close() {
    this.finishing = true;
    this.socket.close();
  }

  async *frames(): AsyncIterable<TranscriberFrame> {
    try {
      for await (const message of this.inbox) {
        if (message.binary) continue;
        yield* this.framesFor(message.data);
        if (this.done) return;
      }
    } finally {
      this.socket.close();
    }
  }

  private framesFor(message: Record<string, unknown>): TranscriberFrame[] {
    const kind = String(message.message_type);
    const text = String(message.text ?? '').trim();
    const now = Math.round(this.ms);
    if (ERRORS.has(kind)) throw new Error(`ElevenLabs ${kind}: ${String(message.error ?? '')}`);
    switch (kind) {
      case 'partial_transcript': {
        if (!text) return [];
        const frames: TranscriberFrame[] = [];
        if (!this.inUtterance) {
          this.inUtterance = true;
          frames.push({ type: 'speech_started', atMs: this.segmentStart });
        }
        frames.push({ type: 'partial', text, startMs: this.segmentStart, endMs: now });
        return frames;
      }
      case 'committed_transcript': {
        const frames: TranscriberFrame[] = text
          ? [{ type: 'final', text, startMs: this.segmentStart, endMs: now }]
          : [];
        // Scribe commits when voice activity stops.
        this.inUtterance = false;
        this.segmentStart = now;
        this.done = this.finishing;
        return [...frames, { type: 'utterance_end' }];
      }
      default:
        return [];
    }
  }

  private chunk(audio: Buffer, commit: boolean) {
    sendJson(this.socket, {
      message_type: 'input_audio_chunk',
      audio_base_64: audio.toString('base64'),
      commit,
      sample_rate: this.rate,
    });
  }
}

export interface ElevenLabsTranscriberOptions {
  /** Defaults to ELEVENLABS_API_KEY. */
  apiKey?: string;
  /** Bounds opening the connection, so a stalled provider is reported; 10 s by default. */
  connectTimeoutMs?: number;
  url?: string;
  modelId?: string;
  options?: Record<string, string>;
}

export function realtimeUrl(audio: AudioStart, options: ElevenLabsTranscriberOptions = {}) {
  if (!supported(audio.sample_rate, audio.channels)) {
    throw new ProviderUnavailableError(`ElevenLabs takes mono PCM at ${PCM_RATES.join(', ')} Hz.`);
  }
  const query = new URLSearchParams({
    model_id: options.modelId ?? 'scribe_v2_realtime',
    audio_format: `pcm_${audio.sample_rate}`,
    commit_strategy: 'vad',
    ...(audio.language ? { language_code: audio.language } : {}),
    ...options.options,
  });
  return `${options.url ?? 'wss://api.elevenlabs.io/v1/speech-to-text/realtime'}?${query}`;
}

/** Transcriber options for ElevenLabs Scribe realtime speech-to-text. */
export function elevenLabsTranscriber(options: ElevenLabsTranscriberOptions = {}): TranscriberTopicOptions {
  return {
    providerAvailable: () => Boolean(keyOf(options.apiKey)),
    openStream: async (audio) => {
      const apiKey = keyOf(options.apiKey);
      if (!apiKey) throw new ProviderUnavailableError('ELEVENLABS_API_KEY is not set.');
      const socket = await dial(realtimeUrl(audio, options), apiKey, ProviderUnavailableError, options.connectTimeoutMs);
      return new ElevenLabsTranscriberStream(socket, audio.sample_rate);
    },
  };
}

/** A transcriber topic on ElevenLabs. */
export function elevenLabsTranscriberTopic(
  options: ElevenLabsTranscriberOptions = {},
  topicOptions: Partial<TranscriberTopicOptions> = {},
) {
  return transcriberTopic({ ...elevenLabsTranscriber(options), ...topicOptions });
}
