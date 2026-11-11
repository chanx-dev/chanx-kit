import WebSocket from 'ws';

import type { AudioStart, TranscriberFrame, TranscriberStream, TranscriberTopicOptions } from '../audio-stream-in';
import { ProviderUnavailableError, transcriberTopic } from '../audio-stream-in';
import { PCM_RATE, keyOf } from './openai';

interface RealtimeEvent {
  type?: string;
  item_id?: string;
  audio_start_ms?: number;
  delta?: string;
  transcript?: string;
  error?: { code?: string; message?: string };
}

/** One realtime transcription session: base64 PCM up, text deltas down. */
export class OpenAITranscriberStream implements TranscriberStream {
  private ms = 0;
  private partials = new Map<string, string>();
  private finishing = false;
  private done = false;
  private queue: RealtimeEvent[] = [];
  private ended = false;
  private failure?: Error;
  private wake?: () => void;

  constructor(private readonly socket: WebSocket) {
    socket.on('message', (data: Buffer) => {
      this.queue.push(JSON.parse(data.toString()) as RealtimeEvent);
      this.wake?.();
    });
    socket.once('close', () => this.end());
    socket.once('error', (error) => this.end(error));
  }

  async send(audio: Buffer) {
    this.event({ type: 'input_audio_buffer.append', audio: audio.toString('base64') });
    this.ms += (audio.length / (PCM_RATE * 2)) * 1000;
  }

  /** Nothing: the session stays open while idle. */
  async keepalive() {}

  async finish() {
    this.finishing = true;
    this.event({ type: 'input_audio_buffer.commit' });
  }

  async close() {
    this.finishing = true;
    this.socket.close();
  }

  async *frames(): AsyncIterable<TranscriberFrame> {
    try {
      for (;;) {
        while (this.queue.length === 0) {
          if (this.ended) {
            if (this.failure && !this.finishing) throw this.failure;
            return;
          }
          await new Promise<void>((r) => (this.wake = r));
        }
        yield* this.framesFor(this.queue.shift()!);
        if (this.done) return;
      }
    } finally {
      this.socket.close();
    }
  }

  private framesFor(event: RealtimeEvent): TranscriberFrame[] {
    const now = Math.round(this.ms);
    const item = event.item_id ?? '';
    switch (event.type) {
      case 'input_audio_buffer.speech_started':
        return [{ type: 'speech_started', atMs: event.audio_start_ms ?? now }];
      case 'conversation.item.input_audio_transcription.delta': {
        this.partials.set(item, (this.partials.get(item) ?? '') + (event.delta ?? ''));
        const text = this.partials.get(item)!.trim();
        return text ? [{ type: 'partial', text, startMs: 0, endMs: now }] : [];
      }
      case 'conversation.item.input_audio_transcription.completed': {
        this.partials.delete(item);
        this.done = this.finishing;
        const text = (event.transcript ?? '').trim();
        // Each committed item is one turn.
        const final: TranscriberFrame[] = text ? [{ type: 'final', text, startMs: 0, endMs: now }] : [];
        return [...final, { type: 'utterance_end' }];
      }
      case 'error':
        // Committing an empty buffer at the end is not a failure.
        if (this.finishing && (event.error?.code ?? '').includes('buffer')) {
          this.done = true;
          return [];
        }
        throw new Error(`OpenAI: ${event.error?.message ?? 'error'}`);
      default:
        return [];
    }
  }

  private event(event: Record<string, unknown>) {
    if (this.socket.readyState === WebSocket.OPEN) this.socket.send(JSON.stringify(event));
  }

  private end(error?: Error) {
    this.failure ??= error;
    this.ended = true;
    this.wake?.();
  }
}

export interface OpenAITranscriberOptions {
  /** Defaults to OPENAI_API_KEY. */
  apiKey?: string;
  url?: string;
  model?: string;
  /**
   * `{ type: 'server_vad' }` ends turns on silence, which barge-in needs; it needs
   * gpt-4o-transcribe or -mini, since gpt-live-transcribe refuses turn detection.
   */
  turnDetection?: Record<string, unknown> | null;
}

export function sessionUpdate(audio: AudioStart, options: OpenAITranscriberOptions = {}) {
  return {
    type: 'session.update',
    session: {
      type: 'transcription',
      audio: {
        input: {
          format: { type: 'audio/pcm', rate: audio.sample_rate },
          transcription: {
            model: options.model ?? 'gpt-live-transcribe',
            ...(audio.language ? { language: audio.language } : {}),
          },
          turn_detection: options.turnDetection ?? null,
        },
      },
    },
  };
}

function dial(url: string, apiKey: string) {
  return new Promise<WebSocket>((resolve, reject) => {
    const socket = new WebSocket(url, { headers: { Authorization: `Bearer ${apiKey}` } });
    socket.once('open', () => resolve(socket));
    socket.once('unexpected-response', (_req, res) => {
      reject(new ProviderUnavailableError(`OpenAI refused the connection (${res.statusCode}).`));
      socket.terminate();
    });
    socket.once('error', (error) => reject(new ProviderUnavailableError(`OpenAI is unreachable: ${error.message}`)));
  });
}

/** Transcriber options for OpenAI realtime transcription. */
export function openAITranscriber(options: OpenAITranscriberOptions = {}): TranscriberTopicOptions {
  return {
    providerAvailable: () => Boolean(keyOf(options.apiKey)),
    openStream: async (audio) => {
      const apiKey = keyOf(options.apiKey);
      if (!apiKey) throw new ProviderUnavailableError('OPENAI_API_KEY is not set.');
      if (audio.sample_rate !== PCM_RATE || audio.channels !== 1) {
        throw new ProviderUnavailableError('OpenAI realtime takes PCM16 mono at 24 kHz.');
      }
      const socket = await dial(options.url ?? 'wss://api.openai.com/v1/realtime?intent=transcription', apiKey);
      const stream = new OpenAITranscriberStream(socket);
      socket.send(JSON.stringify(sessionUpdate(audio, options)));
      return stream;
    },
  };
}

/** A transcriber topic on OpenAI. */
export function openAITranscriberTopic(
  options: OpenAITranscriberOptions = {},
  topicOptions: Partial<TranscriberTopicOptions> = {},
) {
  return transcriberTopic({ ...openAITranscriber(options), ...topicOptions });
}
