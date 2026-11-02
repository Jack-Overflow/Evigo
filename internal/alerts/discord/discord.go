package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/alerts"
)

type Sender struct {
	webhookURL string
	client     *http.Client
}

const (
	username  = "Evigo"
	avatarURL = "https://raw.githubusercontent.com/Jack-Overflow/Evigo/main/docs/evigo-icon.png"
	red       = 0xED4245
	green     = 0x57F287

	titleLimit       = 256
	descriptionLimit = 4096
)

type message struct {
	Username  string  `json:"username"`
	AvatarURL string  `json:"avatar_url"`
	Embeds    []embed `json:"embeds"`
}

type embed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       int    `json:"color"`
	Timestamp   string `json:"timestamp"`
}

func New(webhookURL string) *Sender {
	return &Sender{webhookURL: webhookURL, client: &http.Client{}}
}

func (sender *Sender) Send(ctx context.Context, alert alerts.Alert) error {
	color := red
	if alert.Severity == alerts.Resolved {
		color = green
	}

	body, _ := json.Marshal(message{Username: username, AvatarURL: avatarURL, Embeds: []embed{{
		Title:       shorten(alert.Title, titleLimit),
		Description: shorten(alert.Body, descriptionLimit),
		Color:       color,
		Timestamp:   alert.At.UTC().Format(time.RFC3339),
	}}})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, sender.webhookURL, bytes.NewReader(body))
	if err != nil {
		return errors.New("discord: invalid webhook URL")
	}

	// Without wait, Discord can accept a message it then fails to save.
	query := request.URL.Query()
	query.Set("wait", "true")
	request.URL.RawQuery = query.Encode()
	request.Header.Set("Content-Type", "application/json")
	// The client's error repeats the webhook URL, and its token lets anyone post to the channel.
	response, err := sender.client.Do(request)
	if err != nil {
		return errors.New("discord: cannot reach webhook")
	}

	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("discord: webhook returned status %d", response.StatusCode)
	}

	return nil
}

func shorten(text string, limit int) string {
	characters := []rune(text)
	if len(characters) <= limit {
		return text
	}

	return string(characters[:limit-1]) + "…"
}
