# ag-ui

Serve the [AG-UI protocol](https://ag-ui.com) over a chanx websocket, so any AG-UI
frontend works unchanged. Speaks **ag-ui@2**, like the Python kit, so the `ag-ui` UI kit
works against it. Needs chanx-go 2.2 or newer.

```bash
copit add @chanx-kit-go/ag-ui
```

## Use it

Give the topic your agent's content events, then list it on a consumer:

```go
func (AgentConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{
		agui.AgUiTopic{RunAgent: func(ctx context.Context, in agui.RunAgentInput) iter.Seq2[agui.Event, error] {
			return func(yield func(agui.Event, error) bool) {
				for delta, err := range myAgent(ctx, in.Messages) {
					if !yield(agui.TextMessageContent("m1", delta), err) || err != nil {
						return
					}
				}
			}
		}},
		agui.AgUiRunTopic{},
	}
}
```

A client subscribes to `agui:thread:<thread_id>` and sends `ag_ui_run`. `RUN_STARTED`
and `RUN_FINISHED` bracket what you yield, and an error becomes `RUN_ERROR`. Set
`RunEvents` instead when your provider emits its own lifecycle. A stream should stop
when `ctx` is done: that is how a cancel reaches it.

## One run at a time

A thread runs one run at a time. A second `ag_ui_run` while one is in flight gets a
`RUN_ERROR` naming the run holding the thread, sent to the asking connection only.
`ag_ui_cancel` with the `runId` stops it, and it ends as `RUN_ERROR` "Run cancelled.".

## Several tabs on one thread

With `Broadcast: true` every connection on the thread sees the run, numbered by `seq`,
and one joining part-way is replayed the run so far. Runs then outlive the tab that
started them, and the server can start one itself:

```go
topic := agui.AgUiTopic{RunAgent: run, Broadcast: true}
runID, _ := topic.StartRun(ctx, engine.Publisher(), "thread-1", agui.RunAgentInput{Messages: msgs})
topic.EmitToThread(ctx, engine.Publisher(), "thread-1", event)
agui.EmitToRun(ctx, engine.Publisher(), runID, event) // from a worker, by run id
```

Without `Broadcast`, a run is its socket's: leaving the thread stops it.

## Customise

| Field | Default | Purpose |
|---|---|---|
| `RunAgent` | none | Content events of a run |
| `RunEvents` | lifecycle around `RunAgent` | The whole stream |
| `Broadcast` | `false` | Share runs across the thread's connections |
| `Transcript` | none | What a reconnecting client is shown first |
| `RunEventStore`, `ActiveRunStore` | in memory | Shared stores for several processes |

## Wire contract

The payload schemas are the Python kit's, in `schema.json` (generated, do not edit),
and frames are validated against them. Events are `agui.Event` maps; `RunStarted`,
`TextMessageContent` and the other constructors cover the common ones.
