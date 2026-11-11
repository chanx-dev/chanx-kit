# media-stream-out

The plumbing for media the server streams out: producers run one at a time per session
and can be cut off as a whole, and what was sent is kept so a connection joining
part-way is replayed. The speech output kit builds on it.

```bash
copit add @chanx-kit-go/media-stream-out
```

## Use it

```go
var jobs mediastreamout.SerialJobs
jobs.Submit("session-1", func(ctx context.Context) error { return speak(ctx, text) })
jobs.Cancel("session-1") // the running job stops, the queued ones are dropped

store := mediastreamout.NewInMemoryReplayStore()
seq, _ := store.Append(ctx, "session-1", message) // replay to late joiners in order
```

`InMemoryReplayStore` is process-local; implement `ReplayStore` against Redis to share
it across servers.
