package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"golang.org/x/oauth2/google"
)

type Push struct {
	Token      string
	Title      string
	Body       string
	Data       map[string]string
	Priority   string // "high" wakes the device for job offers
	FullScreen bool
}

type Sender interface {
	Send(ctx context.Context, p Push) error
	Name() string
}

// LogSender is the local stand-in when no Firebase service account is configured;
// the apps still get every event live over the WebSocket.
type LogSender struct{}

func (LogSender) Name() string { return "log" }
func (LogSender) Send(_ context.Context, p Push) error {
	slog.Info("push (log sender)", "title", p.Title, "priority", p.Priority, "token_suffix", suffix(p.Token))
	return nil
}

func suffix(s string) string {
	if len(s) <= 6 {
		return s
	}
	return s[len(s)-6:]
}

// FCMSender calls the FCM HTTP v1 API with a service-account token.
type FCMSender struct {
	project string
	client  *http.Client
}

func NewFCMSender(ctx context.Context, credentialsFile string) (*FCMSender, error) {
	raw, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, err
	}
	creds, err := google.CredentialsFromJSON(ctx, raw, "https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		return nil, err
	}
	var meta struct {
		ProjectID string `json:"project_id"`
	}
	_ = json.Unmarshal(raw, &meta)
	return &FCMSender{project: meta.ProjectID, client: oauthClient(ctx, creds)}, nil
}

func (f *FCMSender) Name() string { return "fcm" }

// Send uses data-only messages for offers so the Partner app's headless handler can show a
// full-screen intent with Accept/Decline actions; everything else is a normal notification.
func (f *FCMSender) Send(ctx context.Context, p Push) error {
	msg := map[string]any{"token": p.Token, "data": p.Data}
	android := map[string]any{"priority": "NORMAL"}
	if p.Priority == "high" {
		android["priority"] = "HIGH"
		android["ttl"] = "15s"
	}
	if !p.FullScreen {
		msg["notification"] = map[string]string{"title": p.Title, "body": p.Body}
	}
	msg["android"] = android
	body, _ := json.Marshal(map[string]any{"message": msg})
	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", f.project)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("fcm: status %d", resp.StatusCode)
	}
	return nil
}
