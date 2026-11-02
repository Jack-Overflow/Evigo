package discord

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/alerts"
)

type Sender struct {
	webhookURL string
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
	return &Sender{webhookURL: webhookURL}
}

func (sender *Sender) Send(ctx context.Context, alert alerts.Alert) error {
	color := red
	if alert.Severity == alerts.Resolved {
		color = green
	}

	address, err := url.Parse(sender.webhookURL)
	if err != nil {
		return errors.New("discord: invalid webhook URL")
	}

	// Without wait, Discord can accept a message it then fails to save.
	query := address.Query()
	query.Set("wait", "true")
	address.RawQuery = query.Encode()
	err = alerts.PostJSON(ctx, address.String(), nil, message{Username: username, AvatarURL: avatarURL, Embeds: []embed{{
		Title:       shorten(alert.Title, titleLimit),
		Description: shorten(alert.Body, descriptionLimit),
		Color:       color,
		Timestamp:   alert.At.UTC().Format(time.RFC3339),
	}}})
	if err != nil {
		return fmt.Errorf("discord: %w", err)
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
