import { defineMessage } from '@chanx-js/server';
import { z } from 'zod';

import { Base64Data } from '../media-stream-in';

const pcm16 = z.literal('pcm16').default('pcm16');
const sampleRate = z.number().int().gt(0).default(24000);
const channels = z.number().int().gt(0).default(1);

export const AudioConfig = z
  .object({ encoding: pcm16, sample_rate: sampleRate, channels, chunk_ms: z.number().int().gt(0).default(100) })
  .meta({ title: 'AudioConfig' });
export type AudioConfig = z.output<typeof AudioConfig>;

export const AudioStart = z
  .object({ encoding: pcm16, sample_rate: sampleRate, channels, language: z.string().nullish() })
  .meta({ title: 'AudioStart' });
export type AudioStart = z.output<typeof AudioStart>;

export const AudioChunk = z
  .object({ index: z.number().int().gte(0), data: Base64Data })
  .meta({ title: 'AudioChunk' });

export const SpeechStarted = z.object({ at_ms: z.number().int() }).meta({ title: 'SpeechStarted' });
export type SpeechStarted = z.output<typeof SpeechStarted>;

export const Transcript = z
  .object({ utterance_id: z.string(), text: z.string(), start_ms: z.number().int(), end_ms: z.number().int() })
  .meta({ title: 'Transcript' });
export type Transcript = z.output<typeof Transcript>;

export const TranscriptFinal = Transcript.extend({ confidence: z.number().nullish() }).meta({
  title: 'TranscriptFinal',
});
export type TranscriptFinal = z.output<typeof TranscriptFinal>;

export const UtteranceEnd = z.object({ utterance_id: z.string() }).meta({ title: 'UtteranceEnd' });
export type UtteranceEnd = z.output<typeof UtteranceEnd>;

export const TranscriberErrorCode = z
  .enum(['provider_unavailable', 'limit_reached', 'bad_format', 'audio_gap', 'not_started'])
  .meta({ title: 'TranscriberErrorCode' });
export type TranscriberErrorCode = z.output<typeof TranscriberErrorCode>;

export const TranscriberError = z
  .object({ code: TranscriberErrorCode, message: z.string() })
  .meta({ title: 'TranscriberError' });

export const AudioConfigMessage = defineMessage('audio_config', AudioConfig, {
  description: 'Sent on subscribe: capture to this format.',
});
export const AudioConfigRequestMessage = defineMessage('audio_config_request', {
  description: 'Client asks for the capture format again, without resubscribing.',
});
export const AudioStartMessage = defineMessage('audio_start', AudioStart, {
  description: 'Client begins a stream of audio.',
});
export const AudioChunkMessage = defineMessage('audio_chunk', AudioChunk, {
  description: 'A piece of the stream.',
});
export const AudioEndMessage = defineMessage('audio_end', {
  description: 'The stream is over: flush what the provider holds, then close it.',
});
export const SpeechStartedMessage = defineMessage('speech_started', SpeechStarted, {
  description: 'The provider heard speech begin, for barge-in and listening indicators.',
});
export const TranscriptPartialMessage = defineMessage('transcript_partial', Transcript, {
  description: 'The segment being spoken, so far. Replaces the previous partial of the utterance.',
});
export const TranscriptFinalMessage = defineMessage('transcript_final', TranscriptFinal, {
  description: 'A segment that will not change. An utterance is its finals in order.',
});
export const UtteranceEndMessage = defineMessage('utterance_end', UtteranceEnd, {
  description: 'The speaker paused: the utterance is complete.',
});
export const TranscriberErrorMessage = defineMessage('transcriber_error', TranscriberError, {
  description: 'Something went wrong. Every code except audio_gap ends the stream.',
});

export const TRANSCRIPT_EVENTS = [
  SpeechStartedMessage,
  TranscriptPartialMessage,
  TranscriptFinalMessage,
  UtteranceEndMessage,
] as const;
