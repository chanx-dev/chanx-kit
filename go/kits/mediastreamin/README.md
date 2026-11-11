# media-stream-in

The plumbing for media a client streams in: chunks put back in order with gaps reported,
and a bounded real-time intake with keepalive and idle handling. The audio kits build on
it; you rarely use it directly.

```bash
copit add @chanx-kit-go/media-stream-in
```

## Use it

```go
seq := mediastreamin.NewChunkSequencer(8)
ready, gaps := seq.Push(chunk.Index, chunk.Data) // in order, and what never arrived

intake := &mediastreamin.MediaIntake{Sink: provider.Send, OnEnd: provider.Finish, MaxChunks: 50}
intake.Start(ctx)
intake.Feed(audio) // drops the oldest when the provider falls behind
intake.End()
```

`[]byte` fields travel as standard base64, as `encoding/json` already writes them.
