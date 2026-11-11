import { setTimeout as sleep } from 'node:timers/promises';
import { crc32 } from 'node:zlib';

import type { SpeechFormat, Synthesize, SynthesizerTopicOptions } from '../audio-stream-out';
import { synthesizerTopic } from '../audio-stream-out';

/** A tone per word, pitched by the word, then a gap: speech-like pacing. */
export function wordTones(text: string, format: SpeechFormat, wordMs = 180, gapMs = 70): Buffer[] {
  const rate = format.sample_rate;
  const frame = format.channels * 2;
  return text
    .split(/\s+/)
    .filter(Boolean)
    .map((word) => {
      const pitch = 180 + (crc32(word.toLowerCase()) % 220);
      const tone = Math.floor((rate * wordMs) / 1000);
      const fade = Math.max(1, Math.floor(tone / 10));
      const gap = Math.floor((rate * gapMs) / 1000);
      const piece = Buffer.alloc((tone + gap) * frame);
      for (let i = 0; i < tone; i += 1) {
        const envelope = Math.min(1, i / fade, (tone - i) / fade);
        const value = Math.trunc(6000 * envelope * Math.sin((2 * Math.PI * pitch * i) / rate));
        for (let c = 0; c < format.channels; c += 1) piece.writeInt16LE(value, i * frame + c * 2);
      }
      return piece;
    });
}

export interface FakeSynthesizerOptions {
  wordMs?: number;
  gapMs?: number;
  /** 1 streams in real time; 0 sends at once. */
  pace?: number;
}

/** Speaks any text as a tone per word. */
export function fakeSynthesize({ wordMs = 180, gapMs = 70, pace = 1 }: FakeSynthesizerOptions = {}): Synthesize {
  return async function* (text, _voice, format, signal) {
    const pause = (wordMs + gapMs) * pace;
    for (const piece of wordTones(text, format, wordMs, gapMs)) {
      yield piece;
      if (pause > 0) await sleep(pause, undefined, { signal }).catch(() => undefined);
      if (signal.aborted) return;
    }
  };
}

/** A synthesizer topic speaking in tones. */
export function fakeSynthesizerTopic(
  options: FakeSynthesizerOptions = {},
  topicOptions: Partial<SynthesizerTopicOptions> = {},
) {
  return synthesizerTopic({ synthesize: fakeSynthesize(options), ...topicOptions });
}
