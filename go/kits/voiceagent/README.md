# voice-agent

Talk to an AG-UI agent: what you say is transcribed and sent as a turn, the reply is
spoken sentence by sentence as it streams, and speaking over it interrupts, keeping only
what was actually heard. Works with the `voice-agent` UI kit.

```bash
copit add @chanx-kit-go/voice-agent
```

## Use it

```go
agent := &voiceagent.Agent{
	Agent:       agui.AgUiTopic{RunAgent: myAgent},
	Synthesizer: fakevoice.DefaultSynthesizer.Topic(),
}

func (AssistantConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{
		agent.TranscriberTopic(fakevoice.DefaultTranscriber.Topic()),
		agent.AgUiTopic(),
		agent.Synthesizer,
	}
}

engine.Handle("/ws/assistant", AssistantConsumer{}, chanx.Sequential())
agent.Publisher = engine.Publisher()
```

One session id names all three topics: `transcribe:<session>`, `agui:thread:<session>`
and `speak:<session>`.

The conversation is kept in `Memory`, in-process by default. Set `OwnHistory` for an
agent that keeps its own, `KeepTalking` to stop speech from interrupting, and `OnTurn` to
filter or route what the user said.
