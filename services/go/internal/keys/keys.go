// Package keys is the single source of truth for Redis key names shared across Go services.
package keys

const (
	ProsGeo    = "pros:geo"  // GEO set of online pros
	ProsSeen   = "pros:seen" // ZSET pro -> last fix ms, drives liveness sweeps
	H3Res      = 8
	LivenessMs = 60_000
)

func Hex(cell string) string        { return "hex:r8:" + cell }    // ZSET pro -> last seen ms
func ProHex(pro string) string      { return "pro:hex:" + pro }    // current cell
func ProState(pro string) string    { return "pro:state:" + pro }  // HASH shift(1/0, set by order-service), skills, rating, jobs_today, idle_since, lat, lng, seen
func ProLast(pro string) string     { return "pro:last:" + pro }   // HASH last accepted fix
func ActivePro(pro string) string   { return "active:pro:" + pro } // order id while on a job
func ProLock(pro string) string     { return "lock:pro:" + pro }   // dispatch lock
func Offer(id string) string        { return "offer:" + id }       // live offer, TTL = offer window
func Forecast(cell string) string   { return "forecast:" + cell }  // demand forecast for a hex
func Channel(name string) string    { return "ch:" + name }        // pub/sub + stream name for a WS channel
func ProChannel(pro string) string  { return "pro:" + pro }
func OrderChannel(id string) string { return "order:" + id }
func HexChannel(cell string) string { return "hex:" + cell }

func OrderACL(order string) string            { return "acl:order:" + order }  // HASH customer, pro (written by the notifier)
func PushTokens(user string) string           { return "push:tokens:" + user } // SET of FCM tokens
func Processed(consumer, event string) string { return "processed:" + consumer + ":" + event }
