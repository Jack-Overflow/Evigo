package ntfy

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Jack-Overflow/Evigo/internal/alerts"
)

const (
	highPriority    = 4
	defaultPriority = 3
)

type Settings struct {
	URL   string
	Topic string
	Token string
}

type Sender struct {
	settings Settings
}

type message struct {
	Topic    string   `json:"topic"`
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Priority int      `json:"priority"`
	Tags     []string `json:"tags"`
}

func New(settings Settings) *Sender {
	return &Sender{settings: settings}
}

func (sender *Sender) Send(ctx context.Context, alert alerts.Alert) error {
	priority, tag := highPriority, "rotating_light"
	if alert.Severity == alerts.Resolved {
		priority, tag = defaultPriority, "white_check_mark"
	}

	header := http.Header{}
	if sender.settings.Token != "" {
		header.Set("Authorization", "Bearer "+sender.settings.Token)
	}

	err := alerts.PostJSON(ctx, sender.settings.URL, header, message{
		Topic:    sender.settings.Topic,
		Title:    alert.Title,
		Message:  alert.Body,
		Priority: priority,
		Tags:     []string{tag},
	})
	if err != nil {
		return fmt.Errorf("ntfy: %w", err)
	}

	return nil
}
