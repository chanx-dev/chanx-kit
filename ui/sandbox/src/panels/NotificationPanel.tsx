import { NotificationFeed, useNotifications } from '../chanx-kit/notification/react';
import { notifications } from '../generated';
import type { Server } from '../demos';

const everyone = notifications.topics.broadcastNotificationTopic.with();

/** The script that sends one from another process, for each sandbox server. */
const SEND: Record<Server, { command: string; user: string }> = {
  python: { command: 'python -m sandbox.send_notification "Build finished"', user: '--user' },
  go: { command: 'go -C go/sandbox run . notify "Build finished"', user: '-user' },
  js: { command: 'npm --prefix js run sandbox:notify -- "Build finished"', user: '--user' },
};

/** notification kit, through the notification UI kit. */
export function NotificationPanel({ who, server }: { who: string; server: Server }) {
  // Two audiences on one connection; the server authorizes each topic.
  const mine = notifications.topics.userNotificationTopic.with({ user_id: who });
  const { items, status, ackAll, dismiss } = useNotifications(notifications, {
    queryParams: { as: who },
    topics: [mine, everyone],
    limit: 20,
  });

  return (
    <div className="stack">
      <NotificationFeed
        items={items}
        status={status}
        onAckAll={ackAll}
        onDismiss={dismiss}
        describeTopic={(topic) => (topic === everyone.topic ? 'everyone' : 'just you')}
      />
      <p className="hint">
        Nothing in the browser sends these, that is the point. Trigger one from a
        separate process. Both need Redis: <code>docker compose up -d</code> and{' '}
        <code>REDIS_URL</code> in <code>.env</code> (see <code>.env.example</code>), since
        an in-memory layer cannot cross processes:
        <br />
        <code>{SEND[server].command}</code>
        <br />
        That one reaches every tab. Add <code>{SEND[server].user} {who}</code> to address this
        connection alone, which the server authorizes per subscription.
      </p>
    </div>
  );
}
