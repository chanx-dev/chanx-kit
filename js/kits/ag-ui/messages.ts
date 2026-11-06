import type { BaseEvent, RunAgentInput } from '@ag-ui/core';
import { EventSchemas, RunAgentInputSchema } from '@ag-ui/core/schemas';
import type { StandardSchemaV1 } from '@chanx-js/server';
import { defineMessage } from '@chanx-js/server';
import { z } from 'zod';

import { contractSchema } from './schema';

const defs = contractSchema.$defs;

/** Validates with `zod`, documents with the Python kit's schema, so both servers agree. */
function contract<T>(zod: z.ZodType<T>, message: string): StandardSchemaV1<unknown, T> {
  const json = () => ({ ...(contractSchema[message] as object), $defs: defs });
  return {
    '~standard': {
      version: 1,
      vendor: 'chanx-kit',
      validate: (value) => zod['~standard'].validate(value) as never,
      jsonSchema: { input: json, output: json },
    },
  };
}

export type AgUiCancel = { runId: string };

export const AgUiRunMessage = defineMessage(
  'ag_ui_run',
  contract<RunAgentInput>(RunAgentInputSchema, 'AgUiRunMessage'),
  { description: "Client asks the agent to run; the payload is AG-UI's own RunAgentInput." },
);

export const AgUiCancelMessage = defineMessage(
  'ag_ui_cancel',
  contract<AgUiCancel>(z.object({ runId: z.string() }), 'AgUiCancelMessage'),
  { description: 'Client asks for the run in flight to stop.' },
);

export const AgUiEventMessage = defineMessage(
  'ag_ui_event',
  contract<BaseEvent>(EventSchemas as z.ZodType<BaseEvent>, 'AgUiEventMessage'),
  { description: "One AG-UI event on its way to the client, as the protocol's Event union." },
);
