import { consumer, createChanx } from '@chanx-js/server';
import { connect } from '@chanx-js/server/testing';
import { describe, expect, it } from 'vitest';

import {
  BroadcastNotificationTopic,
  NotificationAckMessage,
  SubjectNotificationTopic,
  notifyAll,
  notifySubject,
  notifyUser,
  userNotificationTopic,
} from '..';

function setup() {
  const acked: string[] = [];
  const app = consumer('/ws', {
    name: 'app',
    authenticate: ({ url }) => url.searchParams.get('as') ?? '',
    topics: [
      userNotificationTopic({ onAcked: (ids) => acked.push(...ids) }),
      SubjectNotificationTopic,
      BroadcastNotificationTopic,
    ],
  });
  const chanx = createChanx({ consumers: [app], logger: { error() {}, warn() {} } });
  const join = async (user: string) => {
    const client = connect(chanx, `/ws?as=${user}`);
    expect(await client.ready).toBe(true);
    await client.receiveUntil('authentication');
    return client;
  };
  return { chanx, acked, join };
}

describe('notification', () => {
  it('reaches only the addressed user, with defaults filled', async () => {
    const { chanx, join } = setup();
    const ann = await join('ann');
    const bob = await join('bob');
    await ann.subscribe('notification:user:ann');
    await bob.subscribe('notification:user:bob');

    await notifyUser(chanx, 'ann', { title: 'Build finished' });

    const [frame] = await ann.receiveUntil('notification');
    expect(frame).toMatchObject({
      action: 'notification',
      topic: 'notification:user:ann',
      payload: { title: 'Build finished', level: 'info', data: {} },
    });
    expect(frame?.payload).toHaveProperty('id');
    expect(frame?.payload).toHaveProperty('created_at');
    expect(await bob.receiveNothing()).toBe(true);
  });

  it("refuses another user's topic", async () => {
    const { join } = setup();
    const ann = await join('ann');

    const reply = await ann.subscribe('notification:user:bob');

    expect(reply).toMatchObject({
      action: 'error',
      payload: { type: 'https://chanx.dev/errors/unauthorized' },
    });
  });

  it('refuses an anonymous connection', async () => {
    const { join } = setup();
    const anon = await join('');

    expect(await anon.subscribe('notification:user:ann')).toMatchObject({ action: 'error' });
  });

  it('broadcasts to everyone and to a subject', async () => {
    const { chanx, join } = setup();
    const ann = await join('ann');
    await ann.subscribe('notification:all');
    await ann.subscribe('notification:subject:deploys');

    await notifyAll(chanx, { title: 'Maintenance' });
    expect(await ann.receive()).toMatchObject({ topic: 'notification:all' });

    await notifySubject(chanx, 'deploys', { title: 'Shipped', level: 'success' });
    expect(await ann.receive()).toMatchObject({
      topic: 'notification:subject:deploys',
      payload: { level: 'success' },
    });
  });

  it('records and confirms an acknowledgement', async () => {
    const { acked, join } = setup();
    const ann = await join('ann');
    await ann.subscribe('notification:user:ann');

    const reply = await ann.request(NotificationAckMessage.create({ ids: ['n1', 'n2'] }), {
      topic: 'notification:user:ann',
    });

    expect(reply).toMatchObject({ action: 'notification_acked', payload: { ids: ['n1', 'n2'] } });
    expect(acked).toEqual(['n1', 'n2']);
  });
});
