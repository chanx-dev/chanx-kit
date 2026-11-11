import { randomUUID } from 'node:crypto';

import type { BaseEvent, RunAgentInput } from '@ag-ui/core';
import { EventType } from '@ag-ui/core';
import type { Chanx, Connection } from '@chanx-js/server';

import type { AgUiTopicOptions, RunContext } from '../ag-ui';
import { ThreadBusyError, agUiTopic } from '../ag-ui';
import type { TranscriberTopicOptions } from '../audio-stream-in';
import { transcriberTopic } from '../audio-stream-in';
import type { synthesizerTopic } from '../audio-stream-out';
import { heardText } from '../audio-stream-out';
import type { Turn, VoiceMemory } from './memory';
import { InMemoryVoiceMemory } from './memory';
import { SentenceBuffer } from './sentences';

type EventStream = (input: RunAgentInput, ctx: RunContext) => AsyncIterable<BaseEvent>;

export interface VoiceAgentOptions extends Omit<AgUiTopicOptions, 'broadcast'> {
  synthesizer: ReturnType<typeof synthesizerTopic>;
  memory?: VoiceMemory;
  /** For a provider that keeps its own history: only the new turn is sent. */
  ownHistory?: boolean;
  minSentenceChars?: number;
  /** Speaking rate, to trim a reply cut off before its audio was complete. */
  msPerChar?: number;
  /** Speaking over the agent no longer interrupts it. */
  keepTalking?: boolean;
  /** What the user finished saying; responds by default. */
  onTurn?: (session: string, text: string) => Promise<void> | void;
}

const newId = () => randomUUID().replaceAll('-', '');

/** Speaks each reply sentence as it completes, and lets speech interrupt it. */
export function voiceAgent(options: VoiceAgentOptions) {
  const { synthesizer, ownHistory = false, minSentenceChars = 12, msPerChar = 65 } = options;
  const memory = options.memory ?? new InMemoryVoiceMemory();
  // Per session, this process only.
  const spoken = new Map<string, { id: string; text: string }[]>();
  const replies = new Map<string, string>();
  const finals = new WeakMap<Connection<string, unknown>, Map<string, string[]>>();
  const transcribers: ReturnType<typeof transcriberTopic>[] = [];
  let app: Chanx | undefined;

  const chanx = () => {
    if (!app) throw new Error('voiceAgent needs attach(chanx)');
    return app;
  };

  const inner: EventStream =
    options.runEvents ??
    async function* (input, ctx) {
      if (!options.runAgent) throw new Error('voiceAgent needs runAgent or runEvents');
      const ids = { threadId: input.threadId, runId: input.runId };
      yield { type: EventType.RUN_STARTED, ...ids } as BaseEvent;
      yield* options.runAgent(input, ctx);
      yield { type: EventType.RUN_FINISHED, ...ids } as BaseEvent;
    };

  async function remember(session: string, turn: Turn) {
    const turns = [...(await memory.load(session)), turn];
    await memory.save(session, turns);
    return turns;
  }

  async function* runEvents(input: RunAgentInput, ctx: RunContext): AsyncIterable<BaseEvent> {
    const session = input.threadId;
    const buffer = new SentenceBuffer(minSentenceChars);
    const said: { id: string; text: string }[] = [];
    spoken.set(session, said);
    replies.delete(session);
    let reply = '';
    let replyId = newId();
    const say = async (text: string) => {
      said.push({ id: await synthesizer.speak(chanx(), session, text), text });
    };

    for await (const event of inner(input, ctx)) {
      yield event;
      const e = event as BaseEvent & { messageId?: string; delta?: string };
      switch (e.type) {
        case EventType.TEXT_MESSAGE_START:
          replyId = e.messageId ?? replyId;
          break;
        case EventType.TEXT_MESSAGE_CONTENT:
          reply += e.delta ?? '';
          for (const sentence of buffer.feed(e.delta ?? '')) await say(sentence);
          break;
        case EventType.TEXT_MESSAGE_END: {
          const rest = buffer.flush();
          if (rest) await say(rest);
          break;
        }
      }
    }
    // Only an uninterrupted run gets here; interrupt keeps what was heard instead.
    const text = reply.trim();
    if (text && !ctx.signal.aborted) {
      await remember(session, { id: replyId, role: 'assistant', content: text });
      replies.set(session, replyId);
    }
  }

  const topic = agUiTopic({ ...options, broadcast: true, runAgent: undefined, runEvents });

  async function heard(session: string, said: { id: string; text: string }[]) {
    const parts: string[] = [];
    for (const { id, text } of said) {
      const played = await synthesizer.playbackStore.get(session, id);
      if (!played || played.playedMs <= 0) break;
      const duration = played.finished ? played.durationMs : [...text].length * msPerChar;
      if (played.playedMs >= duration) {
        parts.push(text);
        continue;
      }
      const part = heardText(text, duration, played.playedMs);
      if (part) parts.push(part);
      break;
    }
    return parts.join(' ');
  }

  /** Run the agent on the conversation plus `text`; undefined if the session is busy. */
  async function respond(session: string, text: string) {
    const user: Turn = { id: newId(), role: 'user', content: text };
    const turns = await remember(session, user);
    try {
      return await topic.startRun(chanx(), session, {
        runId: newId(),
        state: {},
        messages: ownHistory ? [user] : turns,
        tools: [],
        context: [],
        forwardedProps: {},
      });
    } catch (e) {
      if (e instanceof ThreadBusyError) return undefined;
      throw e;
    }
  }

  /** Barge-in: stop speech and the run, keep what was heard; undefined if quiet. */
  async function interrupt(session: string) {
    const said = spoken.get(session) ?? [];
    spoken.delete(session);
    if (!topic.running(session) && !synthesizer.isSpeaking(session)) return undefined;
    await synthesizer.clear(chanx(), session);
    topic.cancelRun(session);

    const text = await heard(session, said);
    const replyId = replies.get(session);
    replies.delete(session);
    // A finished reply was remembered whole; keep only what was heard.
    const turns = (await memory.load(session)).filter((turn) => turn.id !== replyId);
    if (text) turns.push({ id: replyId ?? newId(), role: 'assistant', content: text });
    await memory.save(session, turns);
    await topic.emitToThread(chanx(), session, {
      type: EventType.CUSTOM,
      name: 'voice_interrupted',
      value: { heard: text },
    } as BaseEvent);
    return text;
  }

  /** Utterances become agent runs; speech over the agent interrupts it. */
  function transcriber(base: TranscriberTopicOptions) {
    const finalsOf = (conn: Connection<string, unknown>, session: string) => {
      let sessions = finals.get(conn);
      if (!sessions) finals.set(conn, (sessions = new Map()));
      let texts = sessions.get(session);
      if (!texts) sessions.set(session, (texts = []));
      return texts;
    };
    const built = transcriberTopic({
      ...base,
      onSpeechStarted: async (scope, event) => {
        await base.onSpeechStarted?.(scope, event);
        if (!options.keepTalking) await interrupt(scope.session);
      },
      onFinal: async (scope, final) => {
        await base.onFinal?.(scope, final);
        finalsOf(scope.conn, scope.session).push(final.text);
      },
      onUtteranceEnd: async (scope, event) => {
        await base.onUtteranceEnd?.(scope, event);
        const texts = finalsOf(scope.conn, scope.session);
        const text = texts.splice(0).join(' ').trim();
        if (!text) return;
        if (options.onTurn) await options.onTurn(scope.session, text);
        else await respond(scope.session, text);
      },
    });
    transcribers.push(built);
    return built;
  }

  return {
    /** The agent's topic: broadcast, so every tab on the session hears the run. */
    topic,
    synthesizer,
    transcriberTopic: transcriber,
    respond,
    interrupt,
    memory,
    /** The app, for server-side runs and speech. */
    attach(chanxApp: Chanx) {
      app = chanxApp;
      for (const t of transcribers) t.attach(chanxApp);
    },
  };
}
