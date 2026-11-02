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
	sender Sender
	queue  chan Alert
	log    *slog.Logger
}

func NewService(sender Sender, log *slog.Logger) *Service {
	service := &Service{sender: sender, queue: make(chan Alert, queueCapacity), log: log}
	if sender != nil {
		go service.sendInOrder()
	}

	return service
}

func (service *Service) Raise(alert Alert) {
	if service.sender == nil {
		return
	}

	select {
	case service.queue <- alert:
	default:
		service.log.Warn("alert queue is full; alert dropped", "alert", alert.Title)
	}
}

func (service *Service) sendInOrder() {
	for alert := range service.queue {
		service.send(alert)
	}
}

func (service *Service) send(alert Alert) {
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	if err := service.sender.Send(ctx, alert); err != nil {
		service.log.Warn("cannot send alert", "alert", alert.Title, "error", err)

		return
	}

	service.log.Info("alert sent", "alert", alert.Title)
}
