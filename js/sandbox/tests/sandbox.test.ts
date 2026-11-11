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

  it('answers speech out loud on the assistant', async () => {
    const c = connect(chanx, '/ws/assistant?as=ann');
    await c.receiveUntil('authentication');
    await c.subscribe('speak:s');
    await c.subscribe('transcribe:s');
    await c.receiveUntil('audio_config');
    const loud = Buffer.alloc(4800);
    for (let i = 0; i < loud.length; i += 2) loud.writeInt16LE(4000, i);
    c.send({ action: 'audio_start', topic: 'transcribe:s', payload: { sample_rate: 24000, channels: 1 } });
    for (let index = 0; index < 20; index += 1) {
      c.send({ action: 'audio_chunk', topic: 'transcribe:s', payload: { index, data: loud.toString('base64') } });
    }
    c.send({ action: 'audio_end', topic: 'transcribe:s' });
    const start = (await c.receiveUntil('audio_start', { timeout: 5000 })).at(-1)!;
    expect((start.payload as { text: string }).text).toBe('You said: "hello world this is a".');
  });

  it('serves every framework', () => {
    expect(Object.keys(frameworks)).toEqual(['node', 'express', 'koa', 'fastify', 'hono']);
  });
});
