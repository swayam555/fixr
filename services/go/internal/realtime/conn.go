package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"fixr.in/svc/internal/platform"
	"github.com/coder/websocket"
)

const (
	sendBuffer     = 256
	pingEvery      = 25 * time.Second
	writeTimeout   = 10 * time.Second
	CloseSlow      = websocket.StatusCode(4008)
	CloseForbidden = websocket.StatusCode(4003)
)

type clientMsg struct {
	Op    string `json:"op"`
	Ch    string `json:"ch"`
	After string `json:"after"`
	ID    string `json:"id"`
}

type serverMsg struct {
	Op    string          `json:"op"`
	Ch    string          `json:"ch,omitempty"`
	Seq   string          `json:"seq,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
	ID    string          `json:"id,omitempty"`
	Error string          `json:"error,omitempty"`
}

type chanState struct {
	syncing bool
	pending []Envelope
	lastSeq string
}

type Conn struct {
	hub    *Hub
	acl    *ACL
	ws     *websocket.Conn
	claims *platform.Claims
	cancel context.CancelFunc

	out    chan []byte
	locSig chan struct{}

	mu        sync.Mutex
	channels  map[string]*chanState
	latestLoc map[string][]byte
	closed    bool
}

type Server struct {
	Hub      *Hub
	ACL      *ACL
	Verifier *platform.Verifier
	Origins  []string
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tok, err := platform.BearerToken(r)
	if err != nil {
		platform.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sign in required")
		return
	}
	claims, err := s.Verifier.Verify(tok)
	if err != nil {
		platform.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired token")
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.Origins, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	ws.SetReadLimit(4096)
	ctx, cancel := context.WithCancel(context.Background())
	c := &Conn{hub: s.Hub, acl: s.ACL, ws: ws, claims: claims, cancel: cancel,
		out: make(chan []byte, sendBuffer), locSig: make(chan struct{}, 1),
		channels: map[string]*chanState{}, latestLoc: map[string][]byte{}}
	connections.Inc()
	defer connections.Dec()
	go c.writeLoop(ctx)
	c.readLoop(ctx)
	c.teardown()
}

func (c *Conn) readLoop(ctx context.Context) {
	defer c.cancel()
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		var m clientMsg
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.Op {
		case "ping":
			c.sendCtl(serverMsg{Op: "pong", ID: m.ID})
		case "sub":
			c.subscribe(ctx, m)
		case "unsub":
			c.unsubscribe(ctx, m.Ch)
		}
	}
}

func (c *Conn) subscribe(ctx context.Context, m clientMsg) {
	if !c.acl.CanSubscribe(ctx, c.claims, m.Ch) {
		c.sendCtl(serverMsg{Op: "error", Ch: m.Ch, ID: m.ID, Error: "forbidden"})
		return
	}
	c.mu.Lock()
	if _, ok := c.channels[m.Ch]; ok {
		c.mu.Unlock()
		c.sendCtl(serverMsg{Op: "subscribed", Ch: m.Ch, ID: m.ID})
		return
	}
	st := &chanState{syncing: true, lastSeq: m.After}
	c.channels[m.Ch] = st
	c.mu.Unlock()

	// Subscribe first, then read history: anything published in between is buffered in st.pending
	// and de-duplicated by sequence, so a resume never loses or reorders a state frame.
	if err := c.hub.add(ctx, m.Ch, c); err != nil {
		c.sendCtl(serverMsg{Op: "error", Ch: m.Ch, ID: m.ID, Error: "unavailable"})
		return
	}
	history, err := c.hub.History(ctx, m.Ch, m.After)
	if err != nil {
		slog.Warn("history", "ch", m.Ch, "err", err)
	}
	c.sendCtl(serverMsg{Op: "subscribed", Ch: m.Ch, ID: m.ID})

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, env := range history {
		c.forwardLocked(st, env)
	}
	for _, env := range st.pending {
		c.forwardLocked(st, env)
	}
	st.pending, st.syncing = nil, false
	if snap := c.acl.Snapshot(ctx, m.Ch); snap != nil {
		c.queueLocLocked(m.Ch, snap)
	}
}

func (c *Conn) unsubscribe(ctx context.Context, ch string) {
	c.mu.Lock()
	_, ok := c.channels[ch]
	delete(c.channels, ch)
	delete(c.latestLoc, ch)
	c.mu.Unlock()
	if ok {
		c.hub.remove(ctx, ch, c)
	}
}

// deliver is called from the hub's pump goroutine and must never block.
func (c *Conn) deliver(env Envelope) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, ok := c.channels[env.Ch]
	if !ok || c.closed {
		return
	}
	if st.syncing {
		st.pending = append(st.pending, env)
		return
	}
	c.forwardLocked(st, env)
}

func (c *Conn) forwardLocked(st *chanState, env Envelope) {
	if env.Seq == "" {
		frame, _ := json.Marshal(serverMsg{Op: "msg", Ch: env.Ch, Data: env.Data})
		c.queueLocLocked(env.Ch, frame)
		return
	}
	if st.lastSeq != "" && !SeqLess(st.lastSeq, env.Seq) {
		dropped.WithLabelValues("duplicate").Inc()
		return
	}
	st.lastSeq = env.Seq
	frame, _ := json.Marshal(serverMsg{Op: "msg", Ch: env.Ch, Seq: env.Seq, Data: env.Data})
	select {
	case c.out <- frame:
	default:
		// a client this far behind would show stale state; make it reconnect and resume instead
		dropped.WithLabelValues("slow_consumer").Inc()
		c.closed = true
		go func() {
			_ = c.ws.Close(CloseSlow, "slow consumer, resume")
			c.cancel()
		}()
	}
}

// queueLocLocked keeps only the newest location per channel; the writer flushes whatever is latest.
func (c *Conn) queueLocLocked(ch string, frame []byte) {
	if _, pending := c.latestLoc[ch]; pending {
		dropped.WithLabelValues("coalesced").Inc()
	}
	c.latestLoc[ch] = frame
	select {
	case c.locSig <- struct{}{}:
	default:
	}
}

func (c *Conn) sendCtl(m serverMsg) {
	b, _ := json.Marshal(m)
	select {
	case c.out <- b:
	default:
	}
}

func (c *Conn) writeLoop(ctx context.Context) {
	ping := time.NewTicker(pingEvery)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-c.out:
			if c.write(ctx, b, "state") != nil {
				c.cancel()
				return
			}
		case <-c.locSig:
			c.mu.Lock()
			frames := make([][]byte, 0, len(c.latestLoc))
			for ch, f := range c.latestLoc {
				frames = append(frames, f)
				delete(c.latestLoc, ch)
			}
			c.mu.Unlock()
			for _, f := range frames {
				if c.write(ctx, f, "location") != nil {
					c.cancel()
					return
				}
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.ws.Ping(pctx)
			cancel()
			if err != nil {
				c.cancel()
				return
			}
		}
	}
}

func (c *Conn) write(ctx context.Context, b []byte, kind string) error {
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	err := c.ws.Write(wctx, websocket.MessageText, b)
	if err == nil {
		framesOut.WithLabelValues(kind).Inc()
	}
	return err
}

func (c *Conn) teardown() {
	c.mu.Lock()
	c.closed = true
	chs := make([]string, 0, len(c.channels))
	for ch := range c.channels {
		chs = append(chs, ch)
	}
	c.channels = map[string]*chanState{}
	c.mu.Unlock()
	for _, ch := range chs {
		c.hub.remove(context.Background(), ch, c)
	}
	err := c.ws.Close(websocket.StatusNormalClosure, "")
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Debug("close", "err", err)
	}
}
