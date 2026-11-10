---
description: "Guide: stream an AI agent to React over AG-UI on a WebSocket, with cancel and shared runs."
---

# Stream an AI agent to the browser

Your agent's output, token by token, in a React chat, over the
[AG-UI protocol](https://ag-ui.com). The run lives on a WebSocket topic rather than an
SSE request, so the user can cancel it, a second tab can watch it, and a worker can add
to it. Bring any model or framework: the kit only needs a stream of events.

## 1. Add the kits

=== "Node"

    ```bash
    copit add @chanx-kit-js/ag-ui @chanx-kit-ui/ag-ui
    ```

=== "Python"

    ```bash
    copit add @chanx-kit/ag-ui @chanx-kit-ui/ag-ui
    ```

    Using Pydantic AI? `@chanx-kit/pydantic-ai-ag-ui` runs your agent as is, tool
    approvals included, and speaks to the same UI kit.

=== "Go"

    ```bash
    copit add @chanx-kit-go/ag-ui @chanx-kit-ui/ag-ui
    ```

## 2. Connect your agent

Yield your agent's text as content events. The kit adds the run's start, finish and
errors, and stops the stream when the user cancels:

=== "Node"

    ```ts
    import { EventType } from '@ag-ui/core';

    import { AgUiRunTopic, agUiTopic } from './ws-kits/ag-ui';

    const AgentTopic = agUiTopic({
      async *runAgent(input, { signal }) {
        for await (const delta of myAgent(input.messages, signal)) {
          yield { type: EventType.TEXT_MESSAGE_CONTENT, messageId: 'm1', delta };
        }
      },
    });

    export const agent = consumer('/ws/agent', {
      name: 'agent',
      authenticate,
      topics: [AgentTopic, AgUiRunTopic],
    });
    ```

=== "Python"

    ```python
    from collections.abc import AsyncIterator

    from ag_ui.core import Event, EventType, RunAgentInput, TextMessageContentEvent

    from app.ws_kits import ag_ui
    from app.ws_kits.ag_ui import AgUiEventMessage, AgUiRunTopic


    class AgUiTopic(ag_ui.AgUiTopic):
        channel_layer_alias = ALIAS

        async def run_agent(self, run_input: RunAgentInput) -> AsyncIterator[Event]:
            async for delta in my_agent(run_input.messages):
                yield TextMessageContentEvent(
                    type=EventType.TEXT_MESSAGE_CONTENT, message_id="m1", delta=delta
                )


    @channel(name="agent", description="An AI agent over AG-UI.")
    class AgentConsumer(AsyncJsonWebsocketConsumer[AgUiEventMessage]):
        channel_layer_alias = ALIAS
        topics: ClassVar[list[type[Topic[Any]]]] = [AgUiTopic, AgUiRunTopic]
    ```

    Route it in `app/main.py` with
    `ws.router.add_websocket_route("/agent", AgentConsumer.as_asgi())`.

=== "Go"

    ```go
    type AgentConsumer struct{ NotificationConsumer }

    func (AgentConsumer) Topics() []chanx.TopicHandler {
    	return []chanx.TopicHandler{agui.AgUiTopic{RunAgent: runAgent}, agui.AgUiRunTopic{}}
    }

    func runAgent(ctx context.Context, in agui.RunAgentInput) iter.Seq2[agui.Event, error] {
    	return func(yield func(agui.Event, error) bool) {
    		for delta, err := range myAgent(ctx, in.Messages) {
    			if !yield(agui.TextMessageContent("m1", delta), err) || err != nil {
    				return
    			}
    		}
    	}
    }

    // next to the other routes:
    engine.Handle("/ws/agent", AgentConsumer{}, chanx.Name("agent"))
    ```

    Stop when `ctx` is done: that is how a cancel reaches your agent.

Then regenerate the typed client with `npm --prefix web run gen`.

## 3. Render the conversation

The UI kit keeps the conversation's state; how it looks is up to you:

```tsx
import { useAgentThread } from '@/chanx-kit/ag-ui/react';
import { agent } from '@/generated';

export function Assistant({ threadId }: { threadId: string }) {
  const { messages, running, error, run, cancel } = useAgentThread(
    agent,
    agent.topics.agUiTopic.with({ thread_id: threadId }),
  );
  return (
    <>
      {messages.map((m) => <p key={m.id}><b>{m.role}</b> {String(m.content ?? '')}</p>)}
      {running ? <button onClick={cancel}>Stop</button> : <Composer onSend={run} />}
      {error && <p role="alert">{error}</p>}
    </>
  );
}
```

`Composer` is your text box. `run(text)` sends the whole conversation, as AG-UI expects.

## Go further

- **Several tabs, one conversation.** Turn on broadcasting and every tab on the thread
  sees the run; a tab that joins late is replayed it. See [ag-ui](kits/ag-ui.md).
- **Runs started by the server.** A worker can start a run or add events to one by its
  id, through the run topic, without a connection.
- **Hear and speak.** The [voice guide](voice.md) puts a transcriber and a synthesizer
  around the same agent.
