export interface Utterance {
  text: string;
  durationMs: number;
  playedMs: number;
  finished: boolean;
}

/** The part of `text` heard after `playedMs`, cut at a word. */
export function heardText(text: string, durationMs: number, playedMs: number): string {
  if (durationMs <= 0) return '';
  if (playedMs >= durationMs) return text;
  const chars = [...text];
  const cut = Math.floor((chars.length * playedMs) / durationMs);
  let head = chars.slice(0, cut).join('');
  if (cut < chars.length && !/\s/.test(chars[cut]!) && head.includes(' ')) {
    head = head.slice(0, head.lastIndexOf(' '));
  }
  return head.trimEnd();
}

/** Records what was said and how much was heard. */
export interface PlaybackStore {
  spoken(session: string, utteranceId: string, text: string): Promise<void>;
  finished(session: string, utteranceId: string, durationMs: number): Promise<void>;
  mark(session: string, utteranceId: string, playedMs: number): Promise<void>;
  get(session: string, utteranceId: string): Promise<Utterance | undefined>;
}

/** Process-local; keeps the last `keep` utterances per session. */
export class InMemoryPlaybackStore implements PlaybackStore {
  private sessions = new Map<string, Map<string, Utterance>>();

  constructor(private readonly keep = 50) {}

  private session(name: string) {
    let utterances = this.sessions.get(name);
    if (!utterances) this.sessions.set(name, (utterances = new Map()));
    return utterances;
  }

  async spoken(session: string, utteranceId: string, text: string) {
    const utterances = this.session(session);
    utterances.delete(utteranceId);
    utterances.set(utteranceId, { text, durationMs: 0, playedMs: 0, finished: false });
    while (utterances.size > this.keep) utterances.delete(utterances.keys().next().value!);
  }

  async finished(session: string, utteranceId: string, durationMs: number) {
    const utterance = this.session(session).get(utteranceId);
    if (utterance) Object.assign(utterance, { durationMs, finished: true });
  }

  async mark(session: string, utteranceId: string, playedMs: number) {
    const utterance = this.session(session).get(utteranceId);
    if (utterance) utterance.playedMs = Math.max(utterance.playedMs, playedMs);
  }

  async get(session: string, utteranceId: string) {
    const utterance = this.session(session).get(utteranceId);
    return utterance && { ...utterance };
  }
}
