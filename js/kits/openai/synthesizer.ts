import type { Synthesize, SynthesizerTopicOptions } from '../audio-stream-out';
import { SynthesizerUnavailableError, synthesizerTopic } from '../audio-stream-out';
import { PCM_RATE, keyOf } from './openai';

export interface OpenAISynthesizerOptions {
  /** Defaults to OPENAI_API_KEY. */
  apiKey?: string;
  /** Bounds opening the connection, so a stalled provider is reported; 10 s by default. */
  connectTimeoutMs?: number;
  url?: string;
  model?: string;
  voice?: string;
  /** Tone, pace, accent. */
  instructions?: string;
}

/** OpenAI text-to-speech as streamed PCM. */
export function openAISynthesize(options: OpenAISynthesizerOptions = {}): Synthesize {
  return async function* (text, voice, format, signal) {
    const apiKey = keyOf(options.apiKey);
    if (!apiKey) throw new SynthesizerUnavailableError('OPENAI_API_KEY is not set.');
    // The pcm response format is fixed: 24 kHz, 16-bit, mono.
    if (format.sample_rate !== PCM_RATE || format.channels !== 1) {
      throw new SynthesizerUnavailableError('OpenAI speech is PCM16 mono at 24 kHz.');
    }
    const body = {
      model: options.model ?? 'gpt-4o-mini-tts',
      voice: voice ?? options.voice ?? 'alloy',
      input: text,
      response_format: 'pcm',
      ...(options.instructions ? { instructions: options.instructions } : {}),
    };
    // Only waiting for the response is bounded, not streaming the audio.
    const timeoutMs = options.connectTimeoutMs ?? 10_000;
    const waiting = new AbortController();
    const timer = setTimeout(() => waiting.abort(), timeoutMs);
    let response: Response;
    try {
      response = await fetch(options.url ?? 'https://api.openai.com/v1/audio/speech', {
        method: 'POST',
        headers: { Authorization: `Bearer ${apiKey}`, 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
        signal: AbortSignal.any([signal, waiting.signal]),
      });
    } catch (error) {
      if (signal.aborted) return;
      if (waiting.signal.aborted) throw new SynthesizerUnavailableError(`OpenAI did not answer within ${timeoutMs / 1000} s.`);
      throw new SynthesizerUnavailableError(`OpenAI is unreachable: ${(error as Error).message}`);
    } finally {
      clearTimeout(timer);
    }
    if (response.status === 401 || response.status === 403) {
      throw new SynthesizerUnavailableError(`OpenAI refused the request (${response.status}).`);
    }
    if (!response.ok) {
      throw new Error(`OpenAI speech failed (${response.status}): ${(await response.text()).slice(0, 200)}`);
    }
    for await (const piece of response.body ?? []) yield piece as Uint8Array;
  };
}

/** A synthesizer topic on OpenAI. */
export function openAISynthesizerTopic(
  options: OpenAISynthesizerOptions = {},
  topicOptions: Partial<SynthesizerTopicOptions> = {},
) {
  return synthesizerTopic({
    synthesize: openAISynthesize(options),
    providerAvailable: () => Boolean(keyOf(options.apiKey)),
    ...topicOptions,
  });
}
