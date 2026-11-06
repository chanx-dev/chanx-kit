# redis-presence-store

Shared presence roster backed by Redis, so presence stays correct across several
processes.

```bash
copit add @chanx-kit-go/redis-presence-store   # also installs: presence
```

## Why

The `presence` kit defaults to an in-memory store, which keeps the roster in one
process. Run two replicas and each reports only the connections it holds. This store
puts that state in Redis.

## Use it

```go
client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

presence.PresenceTopic{Store: redispresencestore.New(client)}
```

Or set it once for every topic without a `Store`:
`presence.DefaultStore = redispresencestore.New(client)`.

## How state is kept

One hash per scope, `chanx-kit:presence:<scope>`, mapping connection id to the member
JSON, as the Python kit keeps it: Python, Go and Node servers sharing a Redis share one
roster. Membership is derived from the hash, so a process that dies cannot leave a
counter wrong, and each write refreshes a TTL (an hour by default) so an abandoned scope
expires.

```go
&redispresencestore.RedisPresenceStore{Client: client, KeyPrefix: "myapp:presence", TTL: 30 * time.Minute}
```

## Trade-offs

- Every join and leave costs a round trip; `Members` costs one `HGETALL`.
- The TTL is a backstop, not a heartbeat: lower it and re-register connections if you
  need faster cleanup after a crash.
- `Add` and `Discard` read then write, so one member's tabs churning on several
  processes at the same instant can duplicate or skip an announcement. The roster
  heals on the next read.
