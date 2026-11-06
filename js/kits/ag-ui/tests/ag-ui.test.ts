import type { BaseEvent } from '@ag-ui/core';
import { EventType } from '@ag-ui/core';
import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { describe, expect, it } from 'vitest';

import type { AgUiTopicOptions } from '..';
import { AgUiRunTopic, agUiTopic } from '..';

const text = (delta: string): BaseEvent =>
  ({ type: EventType.TEXT_MESSAGE_CONTENT, messageId: 'm', delta }) as BaseEvent;

async function* reply() {
  yield text('hi');
  yield text(' there');
}

async function* forever(_input: unknown, { signal }: { signal: AbortSignal }) {
  yield text('working');
  await new Promise((resolve) => signal.addEventListener('abort', resolve));
}

function setup(options: AgUiTopicOptions = { runAgent: reply }) {
  const topic = agUiTopic(options);
  const app = consumer('/ws', { name: 'agent', topics: [topic, AgUiRunTopic] });
  const chanx = createChanx({ consumers: [app], logger: { error() {}, warn() {} } });
  const join = async (thread = 't1') => {
    const client = connect(chanx, '/ws');
    await client.ready;
    await client.subscribe(`agui:thread:${thread}`);
    return client;
  };
  return { topic, chanx, join };
}

const run = (runId = 'r1') => ({
  action: 'ag_ui_run',
  topic: 'agui:thread:t1',
  payload: { threadId: 't1', runId, state: {}, messages: [], tools: [], context: [], forwardedProps: {} },
});
const types = (frames: readonly unknown[]) =>
  frames.map((f) => (f as { payload?: { type?: string } }).payload?.type);

describe('ag-ui', () => {
  it('streams a run inside its lifecycle', async () => {
    const { join } = setup();
    const client = await join();

    client.send(run());

    const frames = [...(await client.receiveUntil('ag_ui_event')), ...(await client.receiveAll())];
    expect(types(frames)).toEqual([
      EventType.RUN_STARTED,
      EventType.TEXT_MESSAGE_CONTENT,
      EventType.TEXT_MESSAGE_CONTENT,
      EventType.RUN_FINISHED,
    ]);
  });

  it('refuses a second run while one is in flight, and cancels on request', async () => {
    const { join } = setup({ runAgent: forever });
    const client = await join();
    client.send(run('r1'));
    await client.receiveUntil('ag_ui_event');

    client.send(run('r2'));
    const [refusal] = (await client.receiveAll()).filter(
      (f) => (f.payload as { type: string }).type === EventType.RUN_ERROR,
    );
    expect(refusal?.payload).toMatchObject({ message: 'Thread is already running r1.' });

    client.send({ action: 'ag_ui_cancel', topic: 'agui:thread:t1', payload: { runId: 'r1' } });
    const ended = await client.receiveAll();
    expect(ended.at(-1)?.payload).toMatchObject({ type: EventType.RUN_ERROR, message: 'Run cancelled.' });
  });

  it('stops a run when its only watcher leaves', async () => {
    const { topic, join } = setup({ runAgent: forever });
    const client = await join();
    client.send(run());
    await client.receiveUntil('ag_ui_event');
    expect(topic.running('t1')).toBe(true);

    await client.unsubscribe('agui:thread:t1');
    await new Promise((resolve) => setTimeout(resolve, 20));

    expect(topic.running('t1')).toBe(false);
  });

  it('broadcasts a run with seq and replays it to a late joiner', async () => {
    const { join } = setup({ runAgent: forever, broadcast: true });
    const first = await join();
    first.send(run());
    const started = await first.receiveUntil('ag_ui_event');
    expect(started.at(-1)).toMatchObject({ seq: 1 });

    const late = await join();
    const replayed = await late.receiveAll();

    expect(types(replayed)).toEqual([EventType.RUN_STARTED, EventType.TEXT_MESSAGE_CONTENT]);
  });

  it('starts a run from the server and emits by run id', async () => {
    const { topic, chanx, join } = setup({ runAgent: reply, broadcast: true });
    const watcher = await join();
    await watcher.subscribe('agui:run:r9');

    await topic.startRun(chanx, 't1', { runId: 'r9', messages: [], state: {}, tools: [], context: [], forwardedProps: {} });
    await AgUiRunTopic.emitToRun(chanx, 'r9', text('from a worker'));

    const frames = await watcher.receiveAll();
    expect(types(frames)).toContain(EventType.RUN_FINISHED);
    expect(frames.some((f) => f.topic === 'agui:run:r9')).toBe(true);
  });
});
