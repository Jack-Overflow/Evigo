package alerts

import (
	"context"
	"log/slog"
	"time"
)

const (
	queueCapacity = 100
	sendTimeout   = 30 * time.Second
)

type Sender interface {
	Send(ctx context.Context, alert Alert) error
}

type Service struct {
	queues map[string]chan Alert
	log    *slog.Logger
}

func NewService(sendersByProvider map[string]Sender, log *slog.Logger) *Service {
	service := &Service{queues: map[string]chan Alert{}, log: log}
	for provider, sender := range sendersByProvider {
		queue := make(chan Alert, queueCapacity)
		service.queues[provider] = queue
		go service.sendInOrder(provider, sender, queue)
	}

	return service
}

func (service *Service) Raise(alert Alert) {
	for provider, queue := range service.queues {
		select {
		case queue <- alert:
		default:
			service.log.Warn("alert queue is full; alert dropped", "provider", provider, "alert", alert.Title)
		}
	}
}

func (service *Service) sendInOrder(provider string, sender Sender, queue chan Alert) {
	for alert := range queue {
		service.send(provider, sender, alert)
	}
}

func (service *Service) send(provider string, sender Sender, alert Alert) {
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	if err := sender.Send(ctx, alert); err != nil {
		service.log.Warn("cannot send alert", "provider", provider, "alert", alert.Title, "error", err)

		return
	}

	service.log.Info("alert sent", "provider", provider, "alert", alert.Title)
}
