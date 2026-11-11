import type { AssistantMessage, UserMessage } from '@ag-ui/core';

export type Turn = UserMessage | AssistantMessage;

/** The conversation, since no client sends it. */
export interface VoiceMemory {
  load(threadId: string): Promise<Turn[]>;
  save(threadId: string, turns: Turn[]): Promise<void>;
}

/** Process-local, bounded per thread. */
export class InMemoryVoiceMemory implements VoiceMemory {
  private threads = new Map<string, Turn[]>();

  constructor(private readonly maxTurns = 100) {}

  async load(threadId: string) {
    return [...(this.threads.get(threadId) ?? [])];
  }

  async save(threadId: string, turns: Turn[]) {
    this.threads.set(threadId, turns.slice(-this.maxTurns));
  }
}
