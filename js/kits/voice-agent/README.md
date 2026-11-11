# voice-agent

Talk to an AG-UI agent: what you say is transcribed and sent as a turn, the reply is
spoken sentence by sentence as it streams, and speaking over it interrupts, keeping only
what was actually heard. Works with the `voice-agent` UI kit.

```bash
copit add @chanx-kit-js/voice-agent
```

## Use it

```ts
import { fakeSynthesizerTopic } from './ws-kits/fake-voice/synthesizer';
import { fakeTranscriber } from './ws-kits/fake-voice/transcriber';
import { voiceAgent } from './ws-kits/voice-agent';

const synthesizer = fakeSynthesizerTopic();
const assistant = voiceAgent({ runAgent: myAgent, synthesizer });
const transcriber = assistant.transcriberTopic(fakeTranscriber());

const voice = consumer('/ws/assistant', {
  name: 'assistant',
  topics: [transcriber, assistant.topic, synthesizer],
});
const chanx = createChanx({ consumers: [voice] });
assistant.attach(chanx);
```

One session id names all three topics: `transcribe:<session>`, `agui:thread:<session>`
and `speak:<session>`.

The conversation is kept in `memory`, in-process by default. Set `ownHistory` for an
agent that keeps its own, `keepTalking` to stop speech from interrupting, and `onTurn` to
filter or route what the user said. `respond(session, text)` and `interrupt(session)` work
from the server too.
