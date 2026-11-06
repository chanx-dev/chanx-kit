import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { describe, expect, it } from 'vitest';

import { ChatSendMessage, InMemoryMessageStore, roomChatTopic } from '..';

function setup() {
  const topic = roomChatTopic({ store: new InMemoryMessageStore(), backlogLimit: 2 });
  const room = consumer('/ws', {
    name: 'room',
    authenticate: ({ url }) => url.searchParams.get('as') ?? '',
    topics: [topic],
  });
  const chanx = createChanx({ consumers: [room], logger: { error() {}, warn() {} } });
  const join = async (user: string) => {
    const client = connect(chanx, `/ws?as=${user}`);
    expect(await client.ready).toBe(true);
    await client.receiveUntil('authentication');
    await client.subscribe('chat:lobby');
    const [backlog] = (await client.receiveUntil('chat_backlog')).slice(-1);
    return { client, entries: (backlog?.payload as { entries: { body: string }[] }).entries };
  };
  return { chanx, topic, join };
}

describe('room-chat', () => {
  it('delivers a sent message with its author', async () => {
    const { join } = setup();
    const ann = await join('ann');
    const bob = await join('bob');

    await ann.client.sendMessage(ChatSendMessage.create({ body: 'hi' }), { topic: 'chat:lobby' });

    const [message] = (await bob.client.receiveUntil('chat_message')).slice(-1);
    expect(message?.payload).toMatchObject({ body: 'hi', room: 'lobby', author: { id: 'ann', name: 'ann' } });
    expect(message?.payload).toHaveProperty('id');
    expect(message?.payload).toHaveProperty('sent_at');
  });

  it('replays the backlog up to the limit', async () => {
    const { chanx, topic, join } = setup();
    for (const body of ['one', 'two', 'three']) {
      await topic.postToRoom(chanx, 'lobby', body, { id: 'bot' });
    }

    const ann = await join('ann');

    expect(ann.entries.map((e) => e.body)).toEqual(['two', 'three']);
  });

  it('honours the limit of a backlog request', async () => {
    const { chanx, topic, join } = setup();
    for (const body of ['one', 'two', 'three']) {
      await topic.postToRoom(chanx, 'lobby', body, { id: 'bot' });
    }
    const ann = await join('ann');

    const reply = await ann.client.request(
      { action: 'chat_backlog_request', payload: { limit: 1 } },
      { topic: 'chat:lobby' },
    );

    expect(reply).toMatchObject({ payload: { entries: [{ body: 'three' }] } });
  });

  it('delivers a post made without a connection', async () => {
    const { chanx, topic, join } = setup();
    const ann = await join('ann');

    await topic.postToRoom(chanx, 'lobby', 'deployed', { id: 'ci' });

    const [message] = (await ann.client.receiveUntil('chat_message')).slice(-1);
    expect(message?.payload).toMatchObject({ author: { id: 'ci' } });
    expect(await topic.history('lobby')).toHaveLength(1);
  });
});
