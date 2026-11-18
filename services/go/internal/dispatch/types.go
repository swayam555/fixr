package dispatch

// OpenOrder is the dispatch view of an order, built from ORDER_SEARCHING events.
type OpenOrder struct {
	OrderID     string   `json:"orderId"`
	ZoneID      string   `json:"zoneId"`
	Lat         float64  `json:"lat"`
	Lng         float64  `json:"lng"`
	Skill       string   `json:"skill"`
	SkillLevel  int      `json:"skillLevel"`
	ServiceName string   `json:"serviceName"`
	PriceBand   string   `json:"priceBand"`
	Tools       []string `json:"tools"`
	EstMinutes  int      `json:"estMinutes"`
	CreatedMs   int64    `json:"createdMs"`
	Address     string   `json:"addressSummary"`
}

type Offer struct {
	OfferID   string    `json:"offerId"`
	OrderID   string    `json:"orderId"`
	ProID     string    `json:"proId"`
	EtaSec    int       `json:"etaSec"`
	EtaSource string    `json:"etaSource"`
	DistanceM int       `json:"distanceM"`
	ExpiresMs int64     `json:"expiresMs"`
	Round     int64     `json:"round"`
	Order     OpenOrder `json:"order"`
}

// Result goes to dispatch.results keyed by order id; order-service applies it to the state machine.
type Result struct {
	EventID string `json:"eventId"`
	Type    string `json:"type"` // ACCEPTED | UNFULFILLED
	OrderID string `json:"orderId"`
	ProID   string `json:"proId,omitempty"`
	OfferID string `json:"offerId,omitempty"`
	EtaSec  int    `json:"etaSec,omitempty"`
	AtMs    int64  `json:"atMs"`
}

// OfferEvent goes to dispatch.offers keyed by pro id; the notifier turns it into WS frames and push.
type OfferEvent struct {
	EventID string `json:"eventId"`
	Type    string `json:"type"` // OFFERED | EXPIRED | DECLINED | ACCEPTED
	Offer   Offer  `json:"offer"`
	AtMs    int64  `json:"atMs"`
}
