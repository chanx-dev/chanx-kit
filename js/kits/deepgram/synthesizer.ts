import type { SpeechFormat, Synthesize, SynthesizerTopicOptions } from '../audio-stream-out';
import { SynthesizerUnavailableError, synthesizerTopic } from '../audio-stream-out';
import { Inbox, dial, keyOf, sendJson } from './deepgram';

/** The sample rates Aura produces as PCM. */
export const PCM_RATES = [8000, 16000, 24000, 32000, 48000];

export interface DeepgramSynthesizerOptions {
  /** Defaults to DEEPGRAM_API_KEY. */
  apiKey?: string;
  url?: string;
  /** The default Aura model. */
  voice?: string;
}

export function speakUrl(voice: string, format: SpeechFormat, url = 'wss://api.deepgram.com/v1/speak') {
  if (!PCM_RATES.includes(format.sample_rate) || format.channels !== 1) {
    throw new SynthesizerUnavailableError(`Deepgram produces mono PCM at ${PCM_RATES.join(', ')} Hz.`);
  }
  return `${url}?${new URLSearchParams({ model: voice, encoding: 'linear16', sample_rate: String(format.sample_rate) })}`;
}

/** Deepgram Aura text-to-speech. */
export function deepgramSynthesize(options: DeepgramSynthesizerOptions = {}): Synthesize {
  return async function* (text, voice, format, signal) {
    const apiKey = keyOf(options.apiKey);
    if (!apiKey) throw new SynthesizerUnavailableError('DEEPGRAM_API_KEY is not set.');
    const url = speakUrl(voice ?? options.voice ?? 'aura-2-thalia-en', format, options.url);
    const socket = await dial(url, apiKey, SynthesizerUnavailableError);
    const inbox = new Inbox(socket, () => signal.aborted);
    const stop = () => socket.close();
    signal.addEventListener('abort', stop, { once: true });
    try {
      sendJson(socket, { type: 'Speak', text });
      sendJson(socket, { type: 'Flush' });
      for await (const message of inbox) {
        if (message.binary) {
          yield message.data;
          continue;
        }
        if (message.data.type === 'Flushed') {
          sendJson(socket, { type: 'Close' });
          return;
        }
        if (message.data.type === 'Warning') throw new Error(`Deepgram: ${message.data.description ?? 'warning'}`);
      }
    } finally {
      signal.removeEventListener('abort', stop);
      socket.close();
    }
  };
}

/** A synthesizer topic on Deepgram. */
export function deepgramSynthesizerTopic(
  options: DeepgramSynthesizerOptions = {},
  topicOptions: Partial<SynthesizerTopicOptions> = {},
) {
  return synthesizerTopic({
    synthesize: deepgramSynthesize(options),
    providerAvailable: () => Boolean(keyOf(options.apiKey)),
    ...topicOptions,
  });
}
