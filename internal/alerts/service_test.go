package alerts

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSender struct {
	mutex    sync.Mutex
	failures int
	sent     []Alert
	attempts int
	notify   chan struct{}
}

func newFakeSender(failures int) *fakeSender {
	return &fakeSender{failures: failures, notify: make(chan struct{}, 100)}
}

func (sender *fakeSender) Send(_ context.Context, alert Alert) error {
	sender.mutex.Lock()
	defer sender.mutex.Unlock()
	defer func() { sender.notify <- struct{}{} }()
	sender.attempts++
	if sender.attempts <= sender.failures {
		return errors.New("relay unavailable")
	}

	sender.sent = append(sender.sent, alert)

	return nil
}

func (sender *fakeSender) waitForAttempts(t *testing.T, count int) {
	t.Helper()
	for range count {
		select {
		case <-sender.notify:
		case <-time.After(5 * time.Second):
			t.Fatal("sender was not called in time")
		}
	}
}

func (sender *fakeSender) sentTitles() []string {
	sender.mutex.Lock()
	defer sender.mutex.Unlock()
	var titles []string
	for _, alert := range sender.sent {
		titles = append(titles, alert.Title)
	}

	return titles
}

func newTestService(sender Sender, logs *bytes.Buffer) *Service {
	return NewService(map[string]Sender{"test": sender}, slog.New(slog.NewTextHandler(logs, nil)))
}

func TestGivenRaisedAlertWhenSenderSucceedsThenItIsSent(t *testing.T) {
	// Given a service with a working sender.
	sender := newFakeSender(0)
	service := newTestService(sender, &bytes.Buffer{})

	// When an alert is raised.
	service.Raise(OnBattery(time.Now(), 90))
	sender.waitForAttempts(t, 1)

	// Then it is sent.
	if titles := sender.sentTitles(); len(titles) != 1 || titles[0] != "UPS on battery" {
		t.Fatalf("sent %v", titles)
	}
}

func TestGivenFailingSenderWhenAlertIsRaisedThenTheFailureIsLoggedAndNotRetried(t *testing.T) {
	// Given a sender that fails.
	var logs syncBuffer
	sender := newFakeSender(1)
	service := NewService(map[string]Sender{"test": sender}, slog.New(slog.NewTextHandler(&logs, nil)))

	// When an alert is raised.
	service.Raise(OnBattery(time.Now(), 90))
	sender.waitForAttempts(t, 1)

	// Then the failure is logged once and nothing is retried.
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "cannot send alert") {
		if time.Now().After(deadline) {
			t.Fatalf("logs %s", logs.String())
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-sender.notify:
		t.Fatal("failed alert was retried")
	case <-time.After(20 * time.Millisecond):
	}
}

type hungSender struct{ started *atomic.Int32 }

func (sender hungSender) Send(ctx context.Context, _ Alert) error {
	sender.started.Add(1)
	<-ctx.Done()

	return ctx.Err()
}

func TestGivenHungSenderWhenMoreAlertsThanTheQueueHoldsAreRaisedThenRaiseNeverBlocksAndOverflowIsLogged(t *testing.T) {
	// Given a service whose sender never finishes.
	var logs syncBuffer
	started := &atomic.Int32{}
	service := NewService(map[string]Sender{"test": hungSender{started: started}}, slog.New(slog.NewTextHandler(&logs, nil)))

	// When more alerts are raised than the queue holds.
	finished := make(chan struct{})
	go func() {
		for range queueCapacity + 5 {
			service.Raise(OnBattery(time.Now(), 90))
		}
		close(finished)
	}()

	// Then raising returns, overflow is logged, and only one send is in progress.
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("Raise blocked on a hung sender")
	}
	if !strings.Contains(logs.String(), "alert queue is full; alert dropped") {
		t.Fatalf("logs %s", logs.String())
	}
	if count := started.Load(); count > 1 {
		t.Fatalf("%d sends in progress at once", count)
	}
}

func TestGivenSeveralAlertsWhenTheyAreRaisedThenTheyAreSentInOrder(t *testing.T) {
	// Given a service with a working sender.
	sender := newFakeSender(0)
	service := newTestService(sender, &bytes.Buffer{})

	// When a short outage and a recovery are raised together.
	service.Raise(OnBattery(time.Now(), 90))
	service.Raise(MainsRestored(time.Now(), 90))
	service.Raise(DeviceRecovered(time.Now(), "Home server", 1))
	sender.waitForAttempts(t, 3)

	// Then they arrive in the order they happened.
	want := []string{"UPS on battery", "UPS mains power restored", "Home server is back online"}
	if titles := sender.sentTitles(); !slices.Equal(titles, want) {
		t.Fatalf("sent %v", titles)
	}
}

func TestGivenOneProviderHangsWhenAlertIsRaisedThenTheOtherStillSendsIt(t *testing.T) {
	// Given a provider that never finishes and one that works.
	working := newFakeSender(0)
	senders := map[string]Sender{"hung": hungSender{started: &atomic.Int32{}}, "working": working}
	service := NewService(senders, slog.New(slog.DiscardHandler))

	// When an alert is raised.
	service.Raise(OnBattery(time.Now(), 90))
	working.waitForAttempts(t, 1)

	// Then the working provider sends it without waiting for the hung one.
	if titles := working.sentTitles(); len(titles) != 1 || titles[0] != "UPS on battery" {
		t.Fatalf("sent %v", titles)
	}
}

func TestGivenFailingSenderWhenAlertIsRaisedThenTheLogNamesTheProvider(t *testing.T) {
	// Given a Discord sender that fails.
	var logs syncBuffer
	sender := newFakeSender(1)
	service := NewService(map[string]Sender{"discord": sender}, slog.New(slog.NewTextHandler(&logs, nil)))

	// When an alert is raised.
	service.Raise(OnBattery(time.Now(), 90))
	sender.waitForAttempts(t, 1)

	// Then the failure says which provider it came from.
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "provider=discord") {
		if time.Now().After(deadline) {
			t.Fatalf("logs %s", logs.String())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestGivenNoSenderWhenAlertIsRaisedThenNothingHappens(t *testing.T) {
	// Given a service with no provider configured.
	service := NewService(nil, slog.New(slog.DiscardHandler))

	// When an alert is raised.
	service.Raise(OnBattery(time.Now(), 90))

	// Then it returns without panicking.
}

func TestGivenDeviceTimedOutWithoutAWakeWhenAlertIsBuiltThenItSaysNoWakeWasSent(t *testing.T) {
	// Given a device that timed out with no wake attempts.

	// When the alert is built.
	alert := DeviceGaveUp(time.Now(), "Home server", 0)

	// Then it says no wake was sent rather than implying a wake failed.
	if !strings.Contains(alert.Body, "No wake was sent") {
		t.Fatalf("body %q", alert.Body)
	}
}

func TestGivenOneWakeAttemptWhenAlertIsBuiltThenTheCountIsSingular(t *testing.T) {
	// Given a device that recovered after one wake attempt.

	// When the alert is built.
	alert := DeviceRecovered(time.Now(), "Home server", 1)

	// Then the count reads correctly and names the device.
	if alert.Body != "Home server answered ping after 1 wake attempt." {
		t.Fatalf("body %q", alert.Body)
	}
}

func TestGivenEachAlertWhenItIsBuiltThenProblemsAndRecoveriesAreToldApart(t *testing.T) {
	for _, scenario := range []struct {
		alert Alert
		want  Severity
	}{
		{OnBattery(time.Now(), 90), Problem},
		{MainsRestored(time.Now(), 90), Resolved},
		{DeviceRecovered(time.Now(), "Home server", 1), Resolved},
		{DeviceGaveUp(time.Now(), "Home server", 3), Problem},
	} {
		t.Run(scenario.alert.Title, func(t *testing.T) {
			// Given one of the alerts.

			// When it is built.

			// Then its severity says whether something went wrong or recovered.
			if scenario.alert.Severity != scenario.want {
				t.Fatalf("severity %d, want %d", scenario.alert.Severity, scenario.want)
			}
		})
	}
}

type syncBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (buffer *syncBuffer) Write(data []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()

	return buffer.buffer.Write(data)
}

func (buffer *syncBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()

	return buffer.buffer.String()
}
