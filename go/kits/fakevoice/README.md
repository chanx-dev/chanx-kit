# fake-voice

A transcriber and a synthesizer that need no provider and no key: speech is transcribed
as a scripted sentence, text is spoken as a tone per word. For demos, UI work and CI.

```bash
copit add @chanx-kit-go/fake-voice              # both
copit add @chanx-kit-go/fake-voice --only stt   # or one part
```

## Use it

```go
func (VoiceConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{
		fakevoice.DefaultTranscriber.Topic(),
		fakevoice.DefaultSynthesizer.Topic(),
	}
}
```

Loud audio becomes one scripted word per 400 ms and a pause ends the utterance, so a
real microphone drives it like a real provider. Change `Script` and the timings on a copy
of `DefaultTranscriber`; set `Pace` to 0 to send speech at once in tests.
