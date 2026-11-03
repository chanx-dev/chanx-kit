import { defineMessage } from '@chanx-js/server';
import { z } from 'zod';

export const PresenceMember = z
  .object({
    id: z.string(),
    name: z.string().nullish(),
    data: z.record(z.string(), z.unknown()).default(() => ({})),
  })
  .meta({ id: 'PresenceMember', title: 'PresenceMember' });

export type PresenceMember = z.output<typeof PresenceMember>;
export type PresenceMemberInput = z.input<typeof PresenceMember>;

export const PresenceStatePayload = z
  .object({ scope: z.string(), members: z.array(PresenceMember) })
  .meta({ title: 'PresenceStatePayload' });

export const PresenceEventPayload = z
  .object({ scope: z.string(), member: PresenceMember })
  .meta({ id: 'PresenceEventPayload', title: 'PresenceEventPayload' });

export const PresenceStateMessage = defineMessage('presence_state', PresenceStatePayload, {
  description: 'Full member list, sent to a connection when it joins.',
});

export const PresenceJoinMessage = defineMessage('presence_join', PresenceEventPayload, {
  description: 'Someone became present in the scope.',
});

export const PresenceLeaveMessage = defineMessage('presence_leave', PresenceEventPayload, {
  description: 'Someone is no longer present in the scope.',
});

export const PresenceRequestMessage = defineMessage('presence_request', {
  description: 'Client asks for the roster again, without reconnecting.',
});
