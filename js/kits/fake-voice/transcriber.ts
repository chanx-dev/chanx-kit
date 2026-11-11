import type { AudioStart, TranscriberFrame, TranscriberStream, TranscriberTopicOptions } from '../audio-stream-in';
import { AudioStart as AudioStartSchema, transcriberTopic } from '../audio-stream-in';

/** RMS of PCM16 audio, sampling every 4th sample. */
export function loudness(audio: Buffer): number {
  const n = Math.floor(audio.length / 2);
  if (n === 0) return 0;
  let sum = 0;
  let count = 0;
  for (let i = 0; i < n; i += 4) {
    const v = audio.readInt16LE(i * 2);
    sum += v * v;
    count += 1;
  }
  return Math.sqrt(sum / count);
}

export interface FakeTranscriberOptions {
  script?: string[];
  msPerWord?: number;
  wordsPerFinal?: number;
  /** Speech RMS in PCM16 units. */
  speechLevel?: number;
  /** The pause that ends an utterance. */
  silenceMs?: number;
}

/** Hears one scripted word per `msPerWord` of loud audio; a pause ends the utterance. */
export class FakeTranscriberStream implements TranscriberStream {
  private readonly script: string[];
  private readonly msPerWord: number;
  private readonly wordsPerFinal: number;
  private readonly speechLevel: number;
  private readonly silenceMs: number;
  private readonly bytesPerMs: number;

  private queue: TranscriberFrame[] = [];
  private wake?: () => void;
  private ended = false;
  private now = 0;
  private next = 0;
  private active = false;
  private loudMs = 0;
  private quietMs = 0;
  private wordsHeard = 0;
  private segment: string[] = [];
  private segmentStart = 0;

  constructor(audio: Partial<AudioStart> = {}, options: FakeTranscriberOptions = {}) {
    this.script = options.script ?? 'hello world this is a scripted transcript'.split(' ');
    this.msPerWord = options.msPerWord ?? 400;
    this.wordsPerFinal = options.wordsPerFinal ?? 4;
    this.speechLevel = options.speechLevel ?? 500;
    this.silenceMs = options.silenceMs ?? 600;
    const { sample_rate, channels } = AudioStartSchema.parse(audio);
    this.bytesPerMs = (sample_rate * channels * 2) / 1000;
  }

  async send(audio: Buffer) {
    const duration = audio.length / this.bytesPerMs;
    const loud = loudness(audio) >= this.speechLevel;
    if (loud && !this.active) {
      this.active = true;
      this.loudMs = this.quietMs = this.wordsHeard = 0;
      this.segmentStart = Math.round(this.now);
      this.put({ type: 'speech_started', atMs: this.segmentStart });
    }
    this.now += duration;
    if (!this.active) return;
    if (loud) {
      this.quietMs = 0;
      this.loudMs += duration;
      while (this.wordsHeard < Math.floor(this.loudMs / this.msPerWord)) this.word();
    } else {
      this.quietMs += duration;
      if (this.wordsHeard > 0 && this.quietMs >= this.silenceMs) this.endUtterance();
    }
  }

  async keepalive() {}

  async finish() {
    if (this.active && this.wordsHeard > 0) this.endUtterance();
    this.stop();
  }

  async close() {
    this.stop();
  }

  async *frames(): AsyncIterable<TranscriberFrame> {
    for (;;) {
      while (this.queue.length === 0) {
        if (this.ended) return;
        await new Promise<void>((r) => (this.wake = r));
      }
      yield this.queue.shift()!;
    }
  }

  private word() {
    this.segment.push(this.script[this.next % this.script.length]!);
    this.next += 1;
    this.wordsHeard += 1;
    if (this.segment.length >= this.wordsPerFinal) {
      this.final();
      return;
    }
    this.put({ type: 'partial', text: this.segment.join(' '), startMs: this.segmentStart, endMs: Math.round(this.now) });
  }

  private endUtterance() {
    if (this.segment.length > 0) this.final();
    this.put({ type: 'utterance_end' });
    this.active = false;
  }

  private final() {
    const endMs = Math.round(this.now);
    this.put({ type: 'final', text: this.segment.join(' '), startMs: this.segmentStart, endMs, confidence: 1 });
    this.segment = [];
    this.segmentStart = endMs;
  }

  private put(frame: TranscriberFrame) {
    if (this.ended) return;
    this.queue.push(frame);
    this.wake?.();
  }

  private stop() {
    this.ended = true;
    this.wake?.();
  }
}

/** Transcriber options for the script. It costs nothing, so nothing is limited. */
export function fakeTranscriber(options: FakeTranscriberOptions = {}): TranscriberTopicOptions {
  return {
    openStream: async (audio) => new FakeTranscriberStream(audio, options),
    maxSessionSeconds: null,
    idleTimeoutSeconds: null,
    maxSessionsPerUser: null,
  };
}

/** A transcriber topic using the script. */
export function fakeTranscriberTopic(
  options: FakeTranscriberOptions = {},
  topicOptions: Partial<TranscriberTopicOptions> = {},
) {
  return transcriberTopic({ ...fakeTranscriber(options), ...topicOptions });
}
