import type { ChatEntry } from './messages';

/** Room history. Implement it against your database; the default is for demos and tests. */
export interface MessageStore {
  append(entry: ChatEntry): Promise<void>;
  /** Up to `limit` recent messages, oldest first. */
  backlog(room: string, limit: number): Promise<ChatEntry[]>;
}

/** Process-local, bounded history: not durable, not shared across processes. */
export class InMemoryMessageStore implements MessageStore {
  private readonly rooms = new Map<string, ChatEntry[]>();

  constructor(private readonly maxPerRoom = 200) {}

  async append(entry: ChatEntry) {
    const entries = [...(this.rooms.get(entry.room) ?? []), entry];
    this.rooms.set(entry.room, entries.slice(-this.maxPerRoom));
  }

  async backlog(room: string, limit: number) {
    return (this.rooms.get(room) ?? []).slice(-limit);
  }

  /** Drop all history, for tests. */
  clear() {
    this.rooms.clear();
  }
}
