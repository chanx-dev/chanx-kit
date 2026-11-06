import type { Chanx, TopicContext, TopicOptions } from '@chanx-js/server';
import { topic } from '@chanx-js/server';

import type { NotificationPayloadInput } from './messages';
import {
  NotificationAckMessage,
  NotificationAckedMessage,
  NotificationMessage,
  notificationPayload,
} from './messages';

export interface NotificationTopicOptions {
  /** Group namespace, shared with any server on the same layer. */
  name?: string;
  /** Record that the client saw these notifications. */
  onAcked?: (ids: string[], ctx: TopicContext<string, unknown, never>) => unknown;
}

export interface UserNotificationTopicOptions<User> extends NotificationTopicOptions {
  /** The connection's user id; by default the user itself, or its `id`. */
  userId?: (user: User) => string | undefined;
}

function defaultUserId(user: unknown): string | undefined {
  if (typeof user === 'string') return user || undefined;
  const id = (user as { id?: unknown } | null | undefined)?.id;
  return id === undefined || id === null ? undefined : String(id);
}

function withAcks<const P extends string>(
  pattern: P,
  description: string,
  defaultName: string,
  options: NotificationTopicOptions & Pick<TopicOptions<P, unknown>, 'authorize'>,
) {
  return topic(pattern, {
    name: options.name ?? defaultName,
    description,
    authorize: options.authorize,
  })
    .on(NotificationAckMessage, { output: NotificationAckedMessage }, async (m, ctx) => {
      await options.onAcked?.(m.payload.ids, ctx);
      return NotificationAckedMessage.create(m.payload);
    })
    .passthrough(NotificationMessage);
}

/** One user's notifications: only that user may subscribe. */
export function userNotificationTopic<User = unknown>(
  options: UserNotificationTopicOptions<User> = {},
) {
  const userId = options.userId ?? defaultUserId;
  return withAcks(
    'notification:user:{user_id}',
    'Notifications addressed to one user.',
    'UserNotificationTopic',
    {
      ...options,
      authorize: ({ params, conn }) => {
        const id = userId(conn.user as User);
        return id !== undefined && id === params.user_id;
      },
    },
  );
}

/** A subject users opt into. */
export function subjectNotificationTopic(options: NotificationTopicOptions = {}) {
  return withAcks(
    'notification:subject:{subject}',
    'Notifications for a subject users opt into.',
    'SubjectNotificationTopic',
    options,
  );
}

/** Everyone subscribed. */
export function broadcastNotificationTopic(options: NotificationTopicOptions = {}) {
  return withAcks(
    'notification:all',
    'Notifications addressed to everyone.',
    'BroadcastNotificationTopic',
    options,
  );
}

export const UserNotificationTopic = userNotificationTopic();
export const SubjectNotificationTopic = subjectNotificationTopic();
export const BroadcastNotificationTopic = broadcastNotificationTopic();

const message = (payload: NotificationPayloadInput) =>
  NotificationMessage.create(notificationPayload(payload));

/** Send a notification to every live connection of one user. */
export function notifyUser(
  chanx: Chanx,
  userId: string,
  payload: NotificationPayloadInput,
  on = UserNotificationTopic,
) {
  return chanx.publish(on, on.with({ user_id: userId }), message(payload));
}

/** Send a notification to everyone subscribed to `subject`. */
export function notifySubject(
  chanx: Chanx,
  subject: string,
  payload: NotificationPayloadInput,
  on = SubjectNotificationTopic,
) {
  return chanx.publish(on, on.with({ subject }), message(payload));
}

/** Send a notification to every subscribed client. */
export function notifyAll(
  chanx: Chanx,
  payload: NotificationPayloadInput,
  on = BroadcastNotificationTopic,
) {
  return chanx.publish(on, on.with({}), message(payload));
}
