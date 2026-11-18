package keys

func OpenOrders(zone string) string { return "dispatch:open:" + zone }     // HASH order id -> order json
func Zones() string                 { return "dispatch:zones" }            // SET zones with open orders
func Pending(order string) string   { return "dispatch:pending:" + order } // offer id while an offer is live
func Excluded(order string) string  { return "dispatch:excl:" + order }    // SET pros who declined/timed out
func OfferData(id string) string    { return "offerdata:" + id }           // offer json, outlives the TTL key
func ZoneLeader(zone string) string { return "dispatch:leader:" + zone }
