import type { SpeechFormat, Synthesize, SynthesizerTopicOptions } from '../audio-stream-out';
import { SynthesizerUnavailableError, synthesizerTopic } from '../audio-stream-out';
import { Inbox, PCM_RATES, dial, keyOf, sendJson, supported } from './elevenlabs';

/** ElevenLabs refuses in-band; say what to change. */
export function refusal(error: string, voice: string) {
  const hints: Record<string, string> = {
    payment_required: `voice ${voice} is not available on this plan; use a premade voice`,
    quota_exceeded: "the account's character quota is used up",
  };
  return `ElevenLabs: ${error}${hints[error] ? ` (${hints[error]})` : ''}`;
}

export interface ElevenLabsSynthesizerOptions {
  /** Defaults to ELEVENLABS_API_KEY. */
  apiKey?: string;
  /** Has a `{voice_id}` placeholder. */
  url?: string;
  /** Sarah by default: premade on every plan. Retired voices such as Rachel cost money. */
  voice?: string;
  modelId?: string;
  voiceSettings?: Record<string, unknown>;
  options?: Record<string, string>;
}

export function streamUrl(voice: string, format: SpeechFormat, options: ElevenLabsSynthesizerOptions = {}) {
  if (!supported(format.sample_rate, format.channels)) {
    throw new SynthesizerUnavailableError(`ElevenLabs produces mono PCM at ${PCM_RATES.join(', ')} Hz.`);
  }
  const query = new URLSearchParams({
    model_id: options.modelId ?? 'eleven_flash_v2_5',
    output_format: `pcm_${format.sample_rate}`,
    ...options.options,
  });
  const base = options.url ?? 'wss://api.elevenlabs.io/v1/text-to-speech/{voice_id}/stream-input';
  return `${base.replace('{voice_id}', encodeURIComponent(voice))}?${query}`;
}

/** ElevenLabs streaming text-to-speech. */
export function elevenLabsSynthesize(options: ElevenLabsSynthesizerOptions = {}): Synthesize {
  return async function* (text, requested, format, signal) {
    const apiKey = keyOf(options.apiKey);
    if (!apiKey) throw new SynthesizerUnavailableError('ELEVENLABS_API_KEY is not set.');
    const voice = requested ?? options.voice ?? 'EXAVITQu4vr4xnSDxMaL';
    const socket = await dial(streamUrl(voice, format, options), apiKey, SynthesizerUnavailableError);
    const inbox = new Inbox(socket, () => signal.aborted);
    const stop = () => socket.close();
    signal.addEventListener('abort', stop, { once: true });
    try {
      // The opening text is a single space; empty text ends the input.
      sendJson(socket, { text: ' ', voice_settings: options.voiceSettings ?? { stability: 0.5, similarity_boost: 0.75 } });
      sendJson(socket, { text: `${text} `, flush: true });
      sendJson(socket, { text: '' });
      for await (const message of inbox) {
        if (message.binary) continue;
        const { error, audio, isFinal } = message.data as { error?: string; audio?: string; isFinal?: boolean };
        if (error) throw new SynthesizerUnavailableError(refusal(error, voice));
        if (audio) yield Buffer.from(audio, 'base64');
        if (isFinal) return;
      }
    } finally {
      signal.removeEventListener('abort', stop);
      socket.close();
    }
  };
}

/** A synthesizer topic on ElevenLabs. */
export function elevenLabsSynthesizerTopic(
  options: ElevenLabsSynthesizerOptions = {},
  topicOptions: Partial<SynthesizerTopicOptions> = {},
) {
  return synthesizerTopic({
    synthesize: elevenLabsSynthesize(options),
    providerAvailable: () => Boolean(keyOf(options.apiKey)),
    ...topicOptions,
  });
}
