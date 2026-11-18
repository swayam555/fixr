// Package realtime moves frames from backend services to WebSocket clients via Redis.
// Durable frames are appended to a capped stream (for resume) and published; ephemeral
// frames such as live locations are only published, because only the latest one matters.
package realtime

import (
	"context"
	"encoding/json"

	"fixr.in/svc/internal/keys"
	"github.com/redis/go-redis/v9"
)

const StreamMaxLen = 200

// Envelope is what travels on Redis pub/sub; the gateway forwards Data untouched.
type Envelope struct {
	Ch   string          `json:"ch"`
	Seq  string          `json:"seq,omitempty"`
	Data json.RawMessage `json:"data"`
}

type Emitter struct{ Redis *redis.Client }

func (e *Emitter) Durable(ctx context.Context, channel string, frame any) (string, error) {
	data, err := json.Marshal(frame)
	if err != nil {
		return "", err
	}
	seq, err := e.Redis.XAdd(ctx, &redis.XAddArgs{
		Stream: keys.Channel(channel), MaxLen: StreamMaxLen, Approx: true, Values: map[string]any{"d": data},
	}).Result()
	if err != nil {
		return "", err
	}
	env, _ := json.Marshal(Envelope{Ch: channel, Seq: seq, Data: data})
	return seq, e.Redis.Publish(ctx, keys.Channel(channel), env).Err()
}

func (e *Emitter) Ephemeral(ctx context.Context, channel string, frame any) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	env, _ := json.Marshal(Envelope{Ch: channel, Data: data})
	return e.Redis.Publish(ctx, keys.Channel(channel), env).Err()
}

// SeqLess compares Redis stream IDs ("ms-n").
func SeqLess(a, b string) bool {
	am, an := splitSeq(a)
	bm, bn := splitSeq(b)
	if am != bm {
		return am < bm
	}
	return an < bn
}

func splitSeq(s string) (uint64, uint64) {
	var ms, n uint64
	i := 0
	for ; i < len(s) && s[i] != '-'; i++ {
		ms = ms*10 + uint64(s[i]-'0')
	}
	for i++; i < len(s); i++ {
		n = n*10 + uint64(s[i]-'0')
	}
	return ms, n
}
