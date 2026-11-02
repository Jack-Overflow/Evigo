package discord

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Jack-Overflow/Evigo/internal/alerts"
)

const webhookPath = "/api/webhooks/123/secret-token"

func TestGivenWebhookAcceptsWhenAlertIsSentThenDiscordReceivesItAsAnEmbed(t *testing.T) {
	// Given a webhook that accepts messages.
	var received message
	var contentType string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		contentType = request.Header.Get("Content-Type")
		json.NewDecoder(request.Body).Decode(&received)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	at := time.Date(2026, 11, 2, 16, 30, 0, 0, time.FixedZone("BST", 3600))

	// When an alert is sent.
	err := New(server.URL+webhookPath).Send(context.Background(), alerts.OnBattery(at, 90))
	// Then Discord receives it from Evigo as one red embed with the title, body and time.
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "application/json" || received.Username != "Evigo" || received.AvatarURL == "" || len(received.Embeds) != 1 {
		t.Fatalf("content type %q, message %+v", contentType, received)
	}
	if got := received.Embeds[0]; got.Title != "UPS on battery" || !strings.Contains(got.Description, "90%") || got.Color != red || got.Timestamp != "2026-11-02T15:30:00Z" {
		t.Fatalf("embed %+v", got)
	}
}

func TestGivenResolvedAlertWhenItIsSentThenTheEmbedIsGreen(t *testing.T) {
	// Given a webhook that accepts messages.
	var received message
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		json.NewDecoder(request.Body).Decode(&received)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	// When mains power returns.
	err := New(server.URL+webhookPath).Send(context.Background(), alerts.MainsRestored(time.Now(), 90))
	// Then the embed is green.
	if err != nil {
		t.Fatal(err)
	}
	if len(received.Embeds) != 1 || received.Embeds[0].Color != green {
		t.Fatalf("message %+v", received)
	}
}

func TestGivenWebhookRejectsWhenAlertIsSentThenTheErrorGivesOnlyTheStatus(t *testing.T) {
	// Given a webhook that has been deleted.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, `{"message": "Unknown Webhook secret-token"}`, http.StatusNotFound)
	}))
	defer server.Close()

	// When an alert is sent.
	err := New(server.URL+webhookPath).Send(context.Background(), alerts.OnBattery(time.Now(), 90))

	// Then the error gives the status code and nothing from the URL or the reply.
	if err == nil || err.Error() != "discord: webhook returned status 404" {
		t.Fatalf("error %v", err)
	}
}

func TestGivenUnreachableWebhookWhenAlertIsSentThenTheErrorHidesTheURL(t *testing.T) {
	// Given a webhook that can't be reached.
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()

	// When an alert is sent.
	err := New(server.URL+webhookPath).Send(context.Background(), alerts.OnBattery(time.Now(), 90))

	// Then the error never contains the webhook token.
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error %v", err)
	}
}

func TestGivenSilentWebhookWhenTheDeadlinePassesThenSendReturns(t *testing.T) {
	// Given a webhook that never replies.
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer server.Close()
	defer close(release)

	// When an alert is sent with a short deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := New(server.URL+webhookPath).Send(ctx, alerts.OnBattery(time.Now(), 90))

	// Then it gives up at the deadline without exposing the token.
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("returned after %s", elapsed)
	}
}

func TestGivenWebhookWithAThreadWhenAlertIsSentThenDiscordIsAskedToConfirmItWasSaved(t *testing.T) {
	// Given a webhook URL that posts into a thread.
	var query url.Values
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query = request.URL.Query()
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// When an alert is sent.
	err := New(server.URL+webhookPath+"?thread_id=42").Send(context.Background(), alerts.OnBattery(time.Now(), 90))
	// Then Discord is asked to wait until the message is saved, and the thread is kept.
	if err != nil {
		t.Fatal(err)
	}
	if query.Get("wait") != "true" || query.Get("thread_id") != "42" {
		t.Fatalf("query %v", query)
	}
}

func TestGivenVeryLongDeviceNameWhenAlertIsSentThenItFitsDiscordLimits(t *testing.T) {
	// Given a webhook and a device name longer than Discord allows in a title or description.
	var received message
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		json.NewDecoder(request.Body).Decode(&received)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	deviceName := strings.Repeat("é", 4096)

	// When Evigo gives up on it.
	err := New(server.URL+webhookPath).Send(context.Background(), alerts.DeviceGaveUp(time.Now(), deviceName, 0))
	// Then the title and description are shortened to fit.
	if err != nil {
		t.Fatal(err)
	}
	got := received.Embeds[0]
	if titleLength := utf8.RuneCountInString(got.Title); titleLength != titleLimit || !strings.HasSuffix(got.Title, "…") {
		t.Fatalf("title has %d characters", titleLength)
	}
	if descriptionLength := utf8.RuneCountInString(got.Description); descriptionLength != descriptionLimit {
		t.Fatalf("description has %d characters", descriptionLength)
	}
}
