package ntfy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/alerts"
)

type publishedMessage struct {
	message       message
	authorization string
}

func newNtfyServer(t *testing.T) (*httptest.Server, *publishedMessage) {
	t.Helper()
	published := &publishedMessage{}
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		published.authorization = request.Header.Get("Authorization")
		json.NewDecoder(request.Body).Decode(&published.message)
	}))
	t.Cleanup(server.Close)

	return server, published
}

func TestGivenProblemAlertWhenItIsSentThenNtfyPublishesItWithHighPriority(t *testing.T) {
	// Given an ntfy server and a topic with an access token.
	server, published := newNtfyServer(t)
	sender := New(Settings{URL: server.URL, Topic: "evigo-alerts", Token: "tk_secret"})

	// When the UPS goes on battery.
	err := sender.Send(context.Background(), alerts.OnBattery(time.Now(), 90))
	// Then it is published to the topic with high priority and the token.
	if err != nil {
		t.Fatal(err)
	}
	got := published.message
	if got.Topic != "evigo-alerts" || got.Title != "UPS on battery" || got.Priority != highPriority || !slices.Equal(got.Tags, []string{"rotating_light"}) {
		t.Fatalf("message %+v", got)
	}
	if published.authorization != "Bearer tk_secret" {
		t.Fatalf("authorization %q", published.authorization)
	}
}

func TestGivenResolvedAlertWhenItIsSentThenNtfyPublishesItWithDefaultPriority(t *testing.T) {
	// Given an ntfy server and a topic without a token.
	server, published := newNtfyServer(t)
	sender := New(Settings{URL: server.URL, Topic: "evigo-alerts"})

	// When mains power returns.
	err := sender.Send(context.Background(), alerts.MainsRestored(time.Now(), 90))
	// Then it is published with default priority and no login.
	if err != nil {
		t.Fatal(err)
	}
	got := published.message
	if got.Priority != defaultPriority || !slices.Equal(got.Tags, []string{"white_check_mark"}) {
		t.Fatalf("message %+v", got)
	}
	if published.authorization != "" {
		t.Fatalf("authorization %q", published.authorization)
	}
}
