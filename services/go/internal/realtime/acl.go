package realtime

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"fixr.in/svc/internal/keys"
	"fixr.in/svc/internal/platform"
	"github.com/redis/go-redis/v9"
)

// ACL decides channel access from JWT claims plus order ownership kept in Redis.
type ACL struct{ Redis *redis.Client }

func (a *ACL) CanSubscribe(ctx context.Context, c *platform.Claims, ch string) bool {
	if c.Role == "OPS" {
		return true
	}
	kind, id, ok := strings.Cut(ch, ":")
	if !ok || id == "" {
		return false
	}
	switch kind {
	case "hex":
		return true
	case "order":
		owners, err := a.Redis.HGetAll(ctx, keys.OrderACL(id)).Result()
		if err != nil {
			return false
		}
		return owners["customer"] == c.Subject || (owners["pro"] != "" && owners["pro"] == c.Subject)
	case "pro":
		if c.Role == "PRO" {
			return id == c.Subject
		}
		// a customer may watch a pro only while that pro is on the customer's order
		order, err := a.Redis.Get(ctx, keys.ActivePro(id)).Result()
		if err != nil {
			return false
		}
		owner, _ := a.Redis.HGet(ctx, keys.OrderACL(order), "customer").Result()
		return owner == c.Subject
	}
	return false
}

// Snapshot gives a new subscriber the pro's last known position so the marker appears immediately.
func (a *ACL) Snapshot(ctx context.Context, ch string) []byte {
	pro, ok := strings.CutPrefix(ch, "pro:")
	if !ok {
		return nil
	}
	s, err := a.Redis.HGetAll(ctx, keys.ProState(pro)).Result()
	if err != nil || s["lat"] == "" {
		return nil
	}
	lat, _ := strconv.ParseFloat(s["lat"], 64)
	lng, _ := strconv.ParseFloat(s["lng"], 64)
	at, _ := strconv.ParseInt(s["seen"], 10, 64)
	data, _ := json.Marshal(map[string]any{"type": "pro.location", "proId": pro, "lat": lat, "lng": lng, "at": at, "snapshot": true})
	b, _ := json.Marshal(serverMsg{Op: "msg", Ch: ch, Data: data})
	return b
}
