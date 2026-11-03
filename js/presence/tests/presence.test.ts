import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { describe, expect, it } from 'vitest';

import { InMemoryPresenceStore, presenceTopic } from '..';

function setup() {
  const topic = presenceTopic({ store: new InMemoryPresenceStore() });
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
    await client.subscribe('presence:lobby');
    const [state] = (await client.receiveUntil('presence_state')).slice(-1);
    return { client, members: (state?.payload as { members: { id: string }[] }).members };
  };
  return { topic, join };
}

const ids = (members: { id: string }[]) => members.map((m) => m.id);

describe('presence', () => {
  it('sends the roster and announces a newcomer', async () => {
    const { join } = setup();
    const ann = await join('ann');
    expect(ids(ann.members)).toEqual(['ann']);
    await ann.client.receiveUntil('presence_join');

    const bob = await join('bob');

    expect(ids(bob.members)).toEqual(['ann', 'bob']);
    expect(await ann.client.receiveUntil('presence_join')).toContainEqual(
      expect.objectContaining({ payload: { scope: 'lobby', member: { id: 'bob', name: 'bob', data: {} } } }),
    );
  });

  it('counts a second tab once', async () => {
    const { join } = setup();
    const bob = await join('bob');
    await bob.client.receiveUntil('presence_join');
    const tab = await join('ann');
    await tab.client.receiveUntil('presence_join');
    await bob.client.receiveUntil('presence_join');

    const second = await join('ann');
    expect(await bob.client.receiveNothing()).toBe(true);
    await second.client.close();
    expect(await bob.client.receiveNothing()).toBe(true);

    await tab.client.unsubscribe('presence:lobby');
    expect(await bob.client.receiveUntil('presence_leave')).toContainEqual(
      expect.objectContaining({ payload: expect.objectContaining({ member: expect.objectContaining({ id: 'ann' }) }) }),
    );
  });

  it('announces the leave on disconnect', async () => {
    const { topic, join } = setup();
    const bob = await join('bob');
    await bob.client.receiveUntil('presence_join');
    const ann = await join('ann');
    await bob.client.receiveUntil('presence_join');

    await ann.client.close();

    await bob.client.receiveUntil('presence_leave');
    expect(ids(await topic.membersOf('lobby'))).toEqual(['bob']);
  });

  it('answers a roster request', async () => {
    const { join } = setup();
    const ann = await join('ann');
    await ann.client.receiveUntil('presence_join');

    const reply = await ann.client.request({ action: 'presence_request', payload: null }, { topic: 'presence:lobby' });

    expect(reply).toMatchObject({ action: 'presence_state', payload: { members: [{ id: 'ann' }] } });
  });

  it('keeps an anonymous connection present by connection', async () => {
    const { join } = setup();

    const anon = await join('');

    expect(anon.members[0]).toMatchObject({ name: 'anonymous' });
  });
});
