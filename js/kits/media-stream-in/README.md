# media-stream-in

The plumbing for media a client streams in: chunks put back in order with gaps reported,
and a bounded real-time intake with keepalive and idle handling. The audio kits build on
it; you rarely use it directly.

```bash
copit add @chanx-kit-js/media-stream-in
```

## Use it

```ts
import { Base64Data, ChunkSequencer, MediaIntake } from './ws-kits/media-stream-in';

const sequencer = new ChunkSequencer(8);
const [ready, gaps] = sequencer.push(chunk.index, chunk.data); // in order, and what never arrived

const intake = new MediaIntake((audio) => provider.send(audio), {
  onEnd: () => provider.finish(),
  maxChunks: 50,
});
intake.start();
intake.feed(audio); // drops the oldest when the provider falls behind
intake.end();
```

`Base64Data` is a zod schema that decodes a base64 field into a `Buffer`; `toBase64`
encodes one back.
