export type TranscriberFrame =
  | { type: 'speech_started'; atMs: number }
  | { type: 'partial'; text: string; startMs: number; endMs: number }
  | { type: 'final'; text: string; startMs: number; endMs: number; confidence?: number | null }
  | { type: 'utterance_end' };

/** Thrown by a provider that refuses, such as without a key. */
export class ProviderUnavailableError extends Error {}

/** One provider stream: audio in, frames out until the provider closes. */
export interface TranscriberStream {
  send(audio: Buffer): Promise<void>;
  /** Called while no audio arrives, so the provider does not time out. */
  keepalive(): Promise<void>;
  /** Flush; `frames()` ends after the last results. */
  finish(): Promise<void>;
  /** Close now, abandoning pending results. */
  close(): Promise<void>;
  frames(): AsyncIterable<TranscriberFrame>;
}
