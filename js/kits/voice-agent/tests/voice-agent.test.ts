import type { BaseEvent, RunAgentInput } from '@ag-ui/core';
import { EventType } from '@ag-ui/core';
import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { describe, expect, it } from 'vitest';

import type { RunContext } from '../../ag-ui';
import type { TranscriberFrame } from '../../audio-stream-in';
import { synthesizerTopic } from '../../audio-stream-out';
import { SentenceBuffer, voiceAgent } from '..';

describe('sentence buffer', () => {
  it('releases sentences as they complete', () => {
    const b = new SentenceBuffer(1);
    expect(b.feed('Hello there. How are')).toEqual(['Hello there.']);
    expect(b.feed(' you? Fine')).toEqual(['How are you?']);
    expect(b.flush()).toBe('Fine');
  });

  it('holds tiny sentences for the next', () => {
    expect(new SentenceBuffer(12).feed('Hi. I am well. ')).toEqual(['Hi. I am well.']);
  });

  it('ends a sentence at a line break', () => {
    expect(new SentenceBuffer(1).feed('first line\nsecond')).toEqual(['first line']);
  });
});

const text = (delta: string): BaseEvent[] => [
  { type: EventType.TEXT_MESSAGE_START, messageId: 'm', role: 'assistant' } as BaseEvent,
  { type: EventType.TEXT_MESSAGE_CONTENT, messageId: 'm', delta } as BaseEvent,
  { type: EventType.TEXT_MESSAGE_END, messageId: 'm' } as BaseEvent,
];

/** Replies to the last message. */
async function* echo(input: RunAgentInput) {
  const last = input.messages.at(-1)?.content;
  yield* text(`You said ${String(last)}. I heard ${input.messages.length} messages.`);
}

/** Streams a reply, then waits to be cancelled. */
async function* hold(_input: RunAgentInput, ctx: RunContext) {
  yield* text('A long reply that keeps going. ').slice(0, 2);
  await new Promise((r) => ctx.signal.addEventListener('abort', r));
}

let sessions = 0;

function setup(run: typeof echo | typeof hold, ownHistory = false) {
  const session = `v${(sessions += 1)}`;
  let current = run;
  const synthesizer = synthesizerTopic({
    synthesize: async function* () {
      yield new Uint8Array(12000);
    },
  });
  const agent = voiceAgent({ runAgent: (input, ctx) => current(input, ctx), synthesizer, ownHistory });
  const chanx = createChanx({ consumers: [consumer('/ws', { name: 'voice', topics: [agent.topic, synthesizer] })] });
  agent.attach(chanx);
  const listen = async () => {
    const c = connect(chanx, '/ws');
    await c.subscribe(`agui:thread:${session}`);
    await c.subscribe(`speak:${session}`);
    return c;
  };
  return { agent, synthesizer, session, listen, use: (next: typeof echo | typeof hold) => (current = next) };
}

type Client = Awaited<ReturnType<ReturnType<typeof setup>['listen']>>;

async function until(c: Client, type: string) {
  for (;;) {
    const frame = (await c.receiveUntil('ag_ui_event')).at(-1)!;
    const event = frame.payload as BaseEvent & Record<string, unknown>;
    if (event.type === type) return event;
  }
}

describe('voice agent', () => {
  it('answers what the user says out loud', async () => {
    const { agent, session, listen } = setup(echo);
    const c = await listen();
    await agent.respond(session, 'hello');
    const start = (await c.receiveUntil('audio_start')).at(-1)!;
    expect((start.payload as { text: string }).text).toBe('You said hello.');
  });

  it('remembers the conversation between turns', async () => {
    const { agent, synthesizer, session, listen } = setup(echo);
    const c = await listen();
    await agent.respond(session, 'one');
    await until(c, EventType.RUN_FINISHED);
    await synthesizer.waitUntilSpoken(session);
    await agent.respond(session, 'two');
    expect((await until(c, EventType.TEXT_MESSAGE_CONTENT)).delta).toBe('You said two. I heard 3 messages.');
  });

  it('sends only the new turn to a provider keeping its own history', async () => {
    const { agent, synthesizer, session, listen } = setup(echo, true);
    const c = await listen();
    await agent.respond(session, 'one');
    await until(c, EventType.RUN_FINISHED);
    await synthesizer.waitUntilSpoken(session);
    await agent.respond(session, 'two');
    expect((await until(c, EventType.TEXT_MESSAGE_CONTENT)).delta).toBe('You said two. I heard 1 messages.');
  });

  it('is interrupted by speech, keeping what was heard', async () => {
    const { agent, synthesizer, session, listen } = setup(hold);
    const c = await listen();
    await agent.respond(session, 'talk');
    const start = (await c.receiveUntil('audio_start')).at(-1)!;
    await synthesizer.playbackStore.mark(session, (start.payload as { utterance_id: string }).utterance_id, 5000);

    expect(await agent.interrupt(session)).toBe('A long reply that keeps going.');
    expect((await until(c, EventType.CUSTOM)).value).toEqual({ heard: 'A long reply that keeps going.' });
    const turns = await agent.memory.load(session);
    expect(turns.map((t) => t.content)).toEqual(['talk', 'A long reply that keeps going.']);
  });

  it('does nothing when interrupting a quiet agent', async () => {
    const { agent, session } = setup(echo);
    expect(await agent.interrupt(session)).toBeUndefined();
  });

  it('keeps the previous reply when interrupting a new run', async () => {
    const { agent, synthesizer, session, listen, use } = setup(echo);
    const c = await listen();
    await agent.respond(session, 'one');
    await until(c, EventType.RUN_FINISHED);
    await synthesizer.waitUntilSpoken(session);

    use(hold);
    await agent.respond(session, 'two');
    await until(c, EventType.TEXT_MESSAGE_CONTENT);
    expect(await agent.interrupt(session)).toBe('');
    const turns = await agent.memory.load(session);
    expect(turns.map((t) => t.content)).toEqual(['one', 'You said one. I heard 1 messages.', 'two']);
  });

  it('answers an utterance heard by the transcriber', async () => {
    const { agent, session } = setup(echo);
    const frames: TranscriberFrame[] = [
      { type: 'final', text: 'hi', startMs: 0, endMs: 100 },
      { type: 'final', text: 'there', startMs: 100, endMs: 200 },
      { type: 'utterance_end' },
    ];
    const transcriber = agent.transcriberTopic({
      openStream: async () => ({
        send: async () => {},
        keepalive: async () => {},
        finish: async () => {},
        close: async () => {},
        frames: async function* () {
          yield* frames;
        },
      }),
    });
    const chanx = createChanx({ consumers: [consumer('/ws', { name: 'voice', topics: [transcriber, agent.topic] })] });
    agent.attach(chanx);
    const c = connect(chanx, '/ws');
    await c.subscribe(`agui:thread:${session}`);
    await c.subscribe(`transcribe:${session}`);
    c.send({ action: 'audio_start', topic: `transcribe:${session}`, payload: { sample_rate: 24000, channels: 1 } });
    expect((await until(c, EventType.TEXT_MESSAGE_CONTENT)).delta).toBe('You said hi there. I heard 1 messages.');
  });
});
