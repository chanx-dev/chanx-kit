import { connect } from '@chanx-js/server/testing';
import { describe, expect, it } from 'vitest';

import { createSandbox } from '../app';
import { frameworks } from '../frameworks';

const chanx = createSandbox();

describe('sandbox', () => {
  it("publishes the UI's channels and topics", () => {
    const channels = Object.keys(chanx.asyncapi().channels);

    expect(channels).toEqual(
      expect.arrayContaining([
        'notifications',
        'notifications_user_notification_topic',
        'room',
        'room_room_chat_topic',
        'room_presence_topic',
      ]),
    );
  });

  it('hands query identity to the kits', async () => {
    const ann = connect(chanx, '/ws/rooms/lobby?as=ann');
    expect(await ann.ready).toBe(true);
    await ann.receiveUntil('authentication');

    await ann.subscribe('presence:lobby');

    const [state] = (await ann.receiveUntil('presence_state')).slice(-1);
    expect(state?.payload).toMatchObject({ members: [{ id: 'ann', name: 'Ann' }] });
  });

  it('serves every framework', () => {
    expect(Object.keys(frameworks)).toEqual(['node', 'express', 'koa', 'fastify', 'hono']);
  });
});
