import { randomUUID } from 'node:crypto';

import { defineMessage } from '@chanx-js/server';
import { z } from 'zod';

export const NotificationLevel = z
  .enum(['info', 'success', 'warning', 'error'])
  .meta({ id: 'NotificationLevel', title: 'NotificationLevel' });

export const NotificationPayload = z
  .object({
    title: z.string(),
    body: z.string().nullish(),
    level: NotificationLevel.default('info'),
    id: z.string().optional(),
    created_at: z.iso.datetime().optional(),
    data: z.record(z.string(), z.unknown()).default(() => ({})),
  })
  .meta({ title: 'NotificationPayload' });

export type NotificationPayloadInput = z.input<typeof NotificationPayload>;

/** The payload as sent: defaults, an id and a time filled. */
export function notificationPayload(input: NotificationPayloadInput) {
  const payload = NotificationPayload.parse(input);
  return {
    ...payload,
    id: payload.id ?? randomUUID().replaceAll('-', ''),
    created_at: payload.created_at ?? new Date().toISOString(),
  };
}

export const NotificationAckPayload = z
  .object({ ids: z.array(z.string()) })
  .meta({ title: 'NotificationAckPayload' });

export const NotificationMessage = defineMessage('notification', NotificationPayload, {
  description: 'A notification delivered to the client.',
});

export const NotificationAckMessage = defineMessage(
  'notification_ack',
  NotificationAckPayload,
  { description: 'Client acknowledges that notifications were seen.' },
);

export const NotificationAckedMessage = defineMessage(
  'notification_acked',
  NotificationAckPayload,
  { description: 'Server confirms an acknowledgement was recorded.' },
);
