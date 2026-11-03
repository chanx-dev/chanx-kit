// Prints the AsyncAPI document of an app hosting one kit's contract topic, for
// scripts/contracts.py to compare with the Python kit's.
import type { TopicLike } from '@chanx-js/server';
import { consumer, createChanx } from '@chanx-js/server';

import { BroadcastNotificationTopic } from '../notification';
import { PresenceTopic } from '../presence';
import { RoomChatTopic } from '../room-chat';

const topics: Record<string, TopicLike> = {
  notification: BroadcastNotificationTopic,
  presence: PresenceTopic,
  'room-chat': RoomChatTopic,
};

const kit = process.argv[2] ?? '';
const contractTopic = topics[kit];
if (!contractTopic) {
  console.error('usage: contract <kit>');
  process.exit(2);
}

const contract = consumer('/ws/contract', { name: 'contract', topics: [contractTopic] });
const chanx = createChanx({ consumers: [contract] });
process.stdout.write(JSON.stringify(chanx.asyncapi({ version: '1' }), null, 2));
