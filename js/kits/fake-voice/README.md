# fake-voice

A transcriber and a synthesizer that need no provider and no key: speech is transcribed
as a scripted sentence, text is spoken as a tone per word. For demos, UI work and CI.

```bash
copit add @chanx-kit-js/fake-voice              # both
copit add @chanx-kit-js/fake-voice --only stt   # or one part
```

## Use it

```ts
import { fakeSynthesizerTopic } from './ws-kits/fake-voice/synthesizer';
import { fakeTranscriberTopic } from './ws-kits/fake-voice/transcriber';

const voice = consumer('/ws/voice', {
  name: 'voice',
  topics: [fakeTranscriberTopic(), fakeSynthesizerTopic()],
});
```

Loud audio becomes one scripted word per 400 ms and a pause ends the utterance, so a
real microphone drives it like a real provider. Pass `script` and the timings to change
it; `{ pace: 0 }` sends speech at once in tests. The second argument takes the topic's
own options, and `fakeTranscriber()` gives them bare, for wrappers such as `voice-agent`.
