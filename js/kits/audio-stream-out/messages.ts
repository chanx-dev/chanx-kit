import { defineMessage } from '@chanx-js/server';
import { z } from 'zod';

import { Base64Text } from '../media-stream-in';

export const SpeechFormat = z
  .object({
    encoding: z.literal('pcm16').default('pcm16'),
    sample_rate: z.number().int().gt(0).default(24000),
    channels: z.number().int().gt(0).default(1),
  })
  .meta({ title: 'SpeechFormat' });
export type SpeechFormat = z.output<typeof SpeechFormat>;

export const Speak = z
  .object({ text: z.string(), voice: z.string().nullish(), utterance_id: z.string().nullish() })
  .meta({ title: 'Speak' });

export const PlaybackMark = z
  .object({ utterance_id: z.string(), played_ms: z.number().int().gte(0) })
  .meta({ title: 'PlaybackMark' });

export const SpeechAudioStart = SpeechFormat.extend({ utterance_id: z.string(), text: z.string() }).meta({
  title: 'SpeechAudioStart',
});
export const SpeechAudioChunk = z
  .object({ utterance_id: z.string(), index: z.number().int().gte(0), data: Base64Text })
  .meta({ title: 'SpeechAudioChunk' });
export const SpeechAudioEnd = z
  .object({ utterance_id: z.string(), duration_ms: z.number().int() })
  .meta({ title: 'SpeechAudioEnd' });
export const SpeechClear = z.object({ utterance_id: z.string().nullish() }).meta({ title: 'SpeechClear' });

export const SynthesizerErrorCode = z
  .enum(['provider_unavailable', 'bad_request', 'provider_failed'])
  .meta({ title: 'SynthesizerErrorCode' });
export type SynthesizerErrorCode = z.output<typeof SynthesizerErrorCode>;
export const SynthesizerError = z
  .object({ code: SynthesizerErrorCode, message: z.string(), utterance_id: z.string().nullish() })
  .meta({ title: 'SynthesizerError' });

export const SpeakMessage = defineMessage('speak', Speak, {
  description: 'Client asks for text to be spoken. Queued behind what is already speaking.',
});
export const SpeakClearMessage = defineMessage('speak_clear', {
  description: 'Client asks to stop: the utterance playing and everything queued.',
});
export const PlaybackMarkMessage = defineMessage('playback_mark', PlaybackMark, {
  description: 'Client reports playback progress, so the server knows what was heard.',
});
export const SpeechAudioStartMessage = defineMessage('audio_start', SpeechAudioStart, {
  title: 'SpeechAudioStartMessage',
  description: 'An utterance begins, in this format.',
});
export const SpeechAudioChunkMessage = defineMessage('audio_chunk', SpeechAudioChunk, {
  title: 'SpeechAudioChunkMessage',
  description: "A piece of the utterance's audio.",
});
export const SpeechAudioEndMessage = defineMessage('audio_end', SpeechAudioEnd, {
  title: 'SpeechAudioEndMessage',
  description: "The utterance's audio is complete.",
});
export const SpeechClearMessage = defineMessage('clear', SpeechClear, {
  title: 'SpeechClearMessage',
  description: 'Stop playing now and drop anything buffered: an interruption.',
});
export const SynthesizerErrorMessage = defineMessage('synthesizer_error', SynthesizerError, {
  description: 'An utterance could not be spoken.',
});

export const SPEECH_EVENTS = [
  SpeechAudioStartMessage,
  SpeechAudioChunkMessage,
  SpeechAudioEndMessage,
  SpeechClearMessage,
  SynthesizerErrorMessage,
] as const;
