import type WebSocket from 'ws';

import type { AudioStart, TranscriberFrame, TranscriberStream, TranscriberTopicOptions } from '../audio-stream-in';
import { ProviderUnavailableError, transcriberTopic } from '../audio-stream-in';
import { Inbox, dial, keyOf, sendJson } from './deepgram';

interface Results {
  is_final?: boolean;
  speech_final?: boolean;
  start?: number;
  duration?: number;
  channel?: { alternatives?: { transcript?: string; confidence?: number }[] };
}

const ms = (seconds: unknown) => Math.round(Number(seconds ?? 0) * 1000);

/** Maps one Deepgram server message to transcriber frames. */
export function framesFrom(message: Record<string, unknown>): TranscriberFrame[] {
  switch (message.type) {
    case 'SpeechStarted':
      return [{ type: 'speech_started', atMs: ms(message.timestamp) }];
    case 'UtteranceEnd':
      return [{ type: 'utterance_end' }];
    case 'Results':
      return results(message as Results);
    default:
      return [];
  }
}

function results(message: Results): TranscriberFrame[] {
  const best = message.channel?.alternatives?.[0] ?? {};
  const text = String(best.transcript ?? '').trim();
  const startMs = ms(message.start);
  const endMs = startMs + ms(message.duration);
  const frames: TranscriberFrame[] = [];
  // Deepgram sends empty results for silence.
  if (text && message.is_final) frames.push({ type: 'final', text, startMs, endMs, confidence: best.confidence });
  else if (text) frames.push({ type: 'partial', text, startMs, endMs });
  // Endpointing: the speaker paused.
  if (message.is_final && message.speech_final) frames.push({ type: 'utterance_end' });
  return frames;
}

/** One Deepgram live connection: binary audio up, JSON results down. */
export class DeepgramStream implements TranscriberStream {
  private closing = false;
  private readonly inbox: Inbox;

  constructor(private readonly socket: WebSocket) {
    this.inbox = new Inbox(socket, () => this.closing);
  }

  async send(audio: Buffer) {
    this.socket.send(audio);
  }

  async keepalive() {
    sendJson(this.socket, { type: 'KeepAlive' });
  }

  /** Finalize flushes; CloseStream sends the rest and closes, which ends frames(). */
  async finish() {
    sendJson(this.socket, { type: 'Finalize' });
    sendJson(this.socket, { type: 'CloseStream' });
  }

  async close() {
    this.closing = true;
    sendJson(this.socket, { type: 'CloseStream' });
    this.socket.close();
  }

  async *frames(): AsyncIterable<TranscriberFrame> {
    for await (const message of this.inbox) {
      if (!message.binary) yield* framesFrom(message.data);
    }
  }
}

export interface DeepgramTranscriberOptions {
  /** Defaults to DEEPGRAM_API_KEY. */
  apiKey?: string;
  /** Bounds opening the connection, so a stalled provider is reported; 10 s by default. */
  connectTimeoutMs?: number;
  url?: string;
  model?: string;
  options?: Record<string, string>;
  utteranceEndMs?: number;
  endpointingMs?: number;
}

export function listenUrl(audio: AudioStart, options: DeepgramTranscriberOptions = {}) {
  const query = new URLSearchParams({
    model: options.model ?? 'nova-3',
    encoding: 'linear16',
    sample_rate: String(audio.sample_rate),
    channels: String(audio.channels),
    interim_results: 'true',
    vad_events: 'true',
    utterance_end_ms: String(options.utteranceEndMs ?? 1000),
    endpointing: String(options.endpointingMs ?? 300),
    ...(audio.language ? { language: audio.language } : {}),
    ...options.options,
  });
  return `${options.url ?? 'wss://api.deepgram.com/v1/listen'}?${query}`;
}

/** Transcriber options for Deepgram live speech-to-text. */
export function deepgramTranscriber(options: DeepgramTranscriberOptions = {}): TranscriberTopicOptions {
  return {
    providerAvailable: () => Boolean(keyOf(options.apiKey)),
    openStream: async (audio) => {
      const apiKey = keyOf(options.apiKey);
      if (!apiKey) throw new ProviderUnavailableError('DEEPGRAM_API_KEY is not set.');
      return new DeepgramStream(await dial(listenUrl(audio, options), apiKey, ProviderUnavailableError, options.connectTimeoutMs));
    },
  };
}

/** A transcriber topic on Deepgram. */
export function deepgramTranscriberTopic(
  options: DeepgramTranscriberOptions = {},
  topicOptions: Partial<TranscriberTopicOptions> = {},
) {
  return transcriberTopic({ ...deepgramTranscriber(options), ...topicOptions });
}
