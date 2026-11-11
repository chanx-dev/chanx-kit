# audio-stream-out

Speak text to a session's listeners from any text-to-speech provider. Speaks
**synthesizer@1**, like the Python kit, so the `player` UI kit works unchanged.
Utterances queue in order, an interruption clears them, late joiners are replayed, and
playback marks record what was actually heard.

```bash
copit add @chanx-kit-go/audio-stream-out
```

## Use it

```go
voice := audiostreamout.SynthesizerTopic{Synthesize: elevenlabs.Synthesizer{}.Synthesize}

func (VoiceConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{voice} }
```

A client subscribes to `speak:<session>` and sends `speak`; everyone on the session gets
`audio_start`, `audio_chunk` and `audio_end`. From the server:

```go
voice.Speak(ctx, engine.Publisher(), "session-1", "Your build finished.", "", "")
voice.Clear(ctx, engine.Publisher(), "session-1") // an interruption
```

The topic's Go type is its broadcast group, so two synthesizers on one server need
different session ids.

## Writing a provider

A `Synthesize` function yields PCM16 audio in the requested format. Wrap
`audiostreamout.ErrProviderUnavailable` when the provider refuses.
