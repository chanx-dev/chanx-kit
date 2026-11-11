// Prints the AsyncAPI document of an app hosting one kit's contract topic, for
// scripts/contracts.py to compare with the Python kit's.
import type { TopicLike } from '@chanx-js/server';
import { consumer, createChanx } from '@chanx-js/server';

import { agUiTopic } from '../kits/ag-ui';
import { transcriberTopic } from '../kits/audio-stream-in';
import { synthesizerTopic } from '../kits/audio-stream-out';
import { deepgramSynthesizerTopic } from '../kits/deepgram/synthesizer';
import { deepgramTranscriberTopic } from '../kits/deepgram/transcriber';
import { elevenLabsSynthesizerTopic } from '../kits/elevenlabs/synthesizer';
import { elevenLabsTranscriberTopic } from '../kits/elevenlabs/transcriber';
import { fakeSynthesizerTopic } from '../kits/fake-voice/synthesizer';
import { fakeTranscriberTopic } from '../kits/fake-voice/transcriber';
import { openAISynthesizerTopic } from '../kits/openai/synthesizer';
import { openAITranscriberTopic } from '../kits/openai/transcriber';
import { BroadcastNotificationTopic } from '../kits/notification';
import { PresenceTopic } from '../kits/presence';
import { RoomChatTopic } from '../kits/room-chat';

const topics: Record<string, TopicLike> = {
  'ag-ui': agUiTopic(),
  'audio-stream-out': synthesizerTopic({ synthesize: async function* () {} }),
  'audio-stream-in': transcriberTopic({ openStream: () => Promise.reject(new Error('contract only')) }),
  'fake-voice transcriber@1': fakeTranscriberTopic(),
  'fake-voice synthesizer@1': fakeSynthesizerTopic(),
  'deepgram transcriber@1': deepgramTranscriberTopic(),
  'deepgram synthesizer@1': deepgramSynthesizerTopic(),
  'elevenlabs transcriber@1': elevenLabsTranscriberTopic(),
  'elevenlabs synthesizer@1': elevenLabsSynthesizerTopic(),
  'openai transcriber@1': openAITranscriberTopic(),
  'openai synthesizer@1': openAISynthesizerTopic(),
  notification: BroadcastNotificationTopic,
  presence: PresenceTopic,
  'room-chat': RoomChatTopic,
};

const [kit = '', contractName = ''] = process.argv.slice(2);
// A provider implementing several contracts is listed once per contract.
const contractTopic = topics[`${kit} ${contractName}`] ?? topics[kit];
if (!contractTopic) {
  console.error('usage: contract <kit> [contract]');
  process.exit(2);
}

const contract = consumer('/ws/contract', { name: 'contract', topics: [contractTopic] });
const chanx = createChanx({ consumers: [contract] });
process.stdout.write(JSON.stringify(chanx.asyncapi({ version: '1' }), null, 2));
