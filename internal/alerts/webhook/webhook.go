package webhook

import (
	"context"
	"fmt"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/alerts"
)

type Sender struct {
	url string
}

type payload struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Severity string `json:"severity"`
	At       string `json:"at"`
}

func New(url string) *Sender {
	return &Sender{url: url}
}

func (sender *Sender) Send(ctx context.Context, alert alerts.Alert) error {
	severity := "problem"
	if alert.Severity == alerts.Resolved {
		severity = "resolved"
	}

	err := alerts.PostJSON(ctx, sender.url, nil, payload{
		Title:    alert.Title,
		Body:     alert.Body,
		Severity: severity,
		At:       alert.At.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("webhook: %w", err)
	}

	return nil
}
