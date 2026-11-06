// Package redispresencestore keeps the presence roster in Redis, in the Python kit's layout.
package redispresencestore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"go.chanx.dev/kit/kits/presence"
)

// RedisPresenceStore is a presence.PresenceStore in Redis. TTL bounds how long a
// roster survives with no writes; every write refreshes it.
type RedisPresenceStore struct {
	Client    redis.Cmdable
	KeyPrefix string
	TTL       time.Duration
}

var _ presence.PresenceStore = (*RedisPresenceStore)(nil)

// New is a store with the Python kit's key prefix and an hour's TTL.
func New(client redis.Cmdable) *RedisPresenceStore {
	return &RedisPresenceStore{Client: client, KeyPrefix: "chanx-kit:presence", TTL: time.Hour}
}

func (s *RedisPresenceStore) key(scope string) string { return s.KeyPrefix + ":" + scope }

func (s *RedisPresenceStore) Add(ctx context.Context, scope, connID string, member presence.PresenceMember) (bool, error) {
	key := s.key(scope)
	existing, err := s.connections(ctx, key)
	if err != nil {
		return false, err
	}
	raw, err := json.Marshal(member)
	if err != nil {
		return false, err
	}
	pipe := s.Client.TxPipeline()
	pipe.HSet(ctx, key, connID, raw)
	pipe.Expire(ctx, key, s.TTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return !present(existing, member.ID), nil
}

func (s *RedisPresenceStore) Discard(ctx context.Context, scope, connID string) (*presence.PresenceMember, error) {
	key := s.key(scope)
	raw, err := s.Client.HGet(ctx, key, connID).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var member presence.PresenceMember
	if err := json.Unmarshal([]byte(raw), &member); err != nil {
		return nil, err
	}
	if err := s.Client.HDel(ctx, key, connID).Err(); err != nil {
		return nil, err
	}
	remaining, err := s.connections(ctx, key)
	if err != nil || present(remaining, member.ID) {
		return nil, err
	}
	return &member, nil
}

func (s *RedisPresenceStore) Members(ctx context.Context, scope string) ([]presence.PresenceMember, error) {
	connections, err := s.connections(ctx, s.key(scope))
	members := []presence.PresenceMember{}
	for _, m := range connections {
		if !present(members, m.ID) {
			members = append(members, m)
		}
	}
	return members, err
}

func (s *RedisPresenceStore) connections(ctx context.Context, key string) ([]presence.PresenceMember, error) {
	raw, err := s.Client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	members := make([]presence.PresenceMember, 0, len(raw))
	for _, payload := range raw {
		var m presence.PresenceMember
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, nil
}

func present(members []presence.PresenceMember, id string) bool {
	for _, m := range members {
		if m.ID == id {
			return true
		}
	}
	return false
}
