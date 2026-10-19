package monitor

import (
	"context"
	"log/slog"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/devices"
	"github.com/Jack-Overflow/Evigo/internal/history"
	"github.com/Jack-Overflow/Evigo/internal/nut"
	"github.com/Jack-Overflow/Evigo/internal/recovery"
	"github.com/Jack-Overflow/Evigo/internal/snapshot"
)

const maxUnsavedEvents = 1000

type UPS interface {
	Read(ctx context.Context) (nut.Reading, error)
}

type Storage interface {
	SavePending(ctx context.Context, pending map[string]recovery.Progress) error
	AppendHistory(ctx context.Context, events []history.Event) error
}

type Settings struct {
	PollInterval          time.Duration
	OnBatteryPollInterval time.Duration
	PingTimeout           time.Duration
	MaxReadingAge         time.Duration
}

type Monitor struct {
	settings         Settings
	ups              UPS
	devices          []*devices.Device
	recovery         *recovery.Recovery
	storage          Storage
	ping             devices.Runner
	now              func() time.Time
	log              *slog.Logger
	saved            map[string]recovery.Progress
	storageOK        bool
	lastStatus       string
	lastReachability map[string]devices.Reachability
	events           []history.Event
	snapshot         atomic.Pointer[snapshot.Snapshot]
}

func New(
	settings Settings,
	ups UPS,
	configured []*devices.Device,
	deviceRecovery *recovery.Recovery,
	storage Storage,
	saved map[string]recovery.Progress,
	ping devices.Runner,
	log *slog.Logger,
) *Monitor {
	monitor := &Monitor{
		settings:  settings,
		ups:       ups,
		devices:   configured,
		recovery:  deviceRecovery,
		storage:   storage,
		saved:     saved,
		storageOK: true,
		ping:      ping,
		now:       time.Now,
		log:       log,
	}
	monitor.publishSnapshot(time.Time{}, nil, nil)

	return monitor
}

func (monitor *Monitor) Snapshot() snapshot.Snapshot {
	return monitor.snapshot.Load().Copy()
}

func (monitor *Monitor) Run(ctx context.Context) error {
	for {
		interval := monitor.Pass(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
	}
}

func (monitor *Monitor) Pass(ctx context.Context) time.Duration {
	reading := monitor.readUPS(ctx)
	reachability := monitor.pingDevices(ctx)
	at := monitor.now()
	monitor.recordReachabilityChanges(at, reachability)
	due := monitor.recovery.Update(recovery.Pass{At: at, Reading: reading, Reachability: reachability})
	monitor.events = append(monitor.events, monitor.recovery.TakeEvents()...)
	if monitor.savePending(ctx) {
		monitor.sendWakes(ctx, due, reading)
	}

	monitor.saveHistory(ctx)
	monitor.publishSnapshot(at, reading, reachability)

	if reading != nil && reading.OnBattery() {
		return monitor.settings.OnBatteryPollInterval
	}

	return monitor.settings.PollInterval
}

func (monitor *Monitor) readUPS(ctx context.Context) *nut.Reading {
	reading, err := monitor.ups.Read(ctx)
	if err != nil {
		if monitor.lastStatus != "unavailable" {
			monitor.log.Warn("UPS reading unavailable", "error", err)
			monitor.recordUPSStatus("unavailable", nil)
		}

		return nil
	}

	status := reading.LogStatus()
	if status != monitor.lastStatus {
		monitor.log.Info("UPS status", "status", status, "battery", reading.Battery)
		monitor.recordUPSStatus(status, &reading.Battery)
	}

	return &reading
}

func (monitor *Monitor) recordUPSStatus(status string, batteryPercent *float64) {
	monitor.lastStatus = status
	monitor.events = append(monitor.events, history.Event{
		At:      monitor.now(),
		Kind:    history.UPSStatus,
		Details: history.Details{Status: status, BatteryPercent: batteryPercent},
	})
}

func (monitor *Monitor) recordReachabilityChanges(at time.Time, reachability map[string]devices.Reachability) {
	for _, device := range monitor.devices {
		previous, seen := monitor.lastReachability[device.ID]
		current := reachability[device.ID]
		if seen && current == previous {
			continue
		}

		monitor.events = append(monitor.events, history.Event{
			At:       at,
			Kind:     history.Reachability,
			DeviceID: device.ID,
			Details:  history.Details{Reachability: current.String()},
		})
	}

	monitor.lastReachability = reachability
}

func (monitor *Monitor) pingDevices(ctx context.Context) map[string]devices.Reachability {
	results := make([]devices.Reachability, len(monitor.devices))
	var group sync.WaitGroup
	for index, device := range monitor.devices {
		group.Go(func() {
			results[index] = device.Ping(ctx, monitor.ping, monitor.settings.PingTimeout)
		})
	}

	group.Wait()
	reachability := make(map[string]devices.Reachability, len(monitor.devices))
	for index, device := range monitor.devices {
		reachability[device.ID] = results[index]
	}

	return reachability
}

func (monitor *Monitor) savePending(ctx context.Context) bool {
	pending := monitor.recovery.Pending()
	if maps.Equal(pending, monitor.saved) {
		return true
	}

	if err := monitor.storage.SavePending(ctx, pending); err != nil {
		monitor.log.Error("cannot save recovery state; waking is paused", "error", err)
		monitor.recovery.StorageFailed()
		monitor.storageOK = false

		return false
	}

	monitor.saved = pending
	monitor.storageOK = true

	return true
}

func (monitor *Monitor) sendWakes(ctx context.Context, due []*devices.Device, reading *nut.Reading) {
	if len(due) == 0 {
		return
	}

	for _, device := range due {
		at := monitor.now()
		if reading == nil || at.Before(reading.At) || at.Sub(reading.At) >= monitor.settings.MaxReadingAge {
			monitor.log.Warn("UPS reading too old to wake devices; retrying later")

			return
		}

		wakeContext, cancel := context.WithDeadline(ctx, reading.At.Add(monitor.settings.MaxReadingAge))
		err := device.Wake(wakeContext)
		cancel()
		event := history.Event{
			At:       monitor.now(),
			Kind:     history.WakeSent,
			DeviceID: device.ID,
			Details:  history.Details{Source: history.Automatic, Attempt: monitor.saved[device.ID].Attempts},
		}
		if err != nil {
			event.Kind = history.WakeFailed
			monitor.events = append(monitor.events, event)
			monitor.log.Error("wake failed", "device", device.ID, "error", err)

			continue
		}

		monitor.events = append(monitor.events, event)
		monitor.recovery.WakeSent(device.ID)
		monitor.log.Info("wake sent", "device", device.ID, "mac", device.MAC.String())
	}
}

func (monitor *Monitor) saveHistory(ctx context.Context) {
	if len(monitor.events) == 0 {
		return
	}

	if err := monitor.storage.AppendHistory(ctx, monitor.events); err != nil {
		monitor.log.Error("cannot save history", "error", err)
		if overflow := len(monitor.events) - maxUnsavedEvents; overflow > 0 {
			monitor.events = monitor.events[overflow:]
		}

		return
	}

	monitor.events = nil
}

func (monitor *Monitor) publishSnapshot(at time.Time, reading *nut.Reading, reachability map[string]devices.Reachability) {
	pending := monitor.recovery.Pending()
	latest := snapshot.Snapshot{
		CheckedAt:        at,
		Reading:          reading,
		SafeSince:        monitor.recovery.SafeSince(),
		StorageOK:        monitor.storageOK,
		RecoverySettings: monitor.recovery.Settings(),
	}
	for _, device := range monitor.devices {
		deviceSnapshot := snapshot.Device{
			Device:       *device,
			Reachability: reachability[device.ID],
			WakeSent:     monitor.recovery.WakeSentTo(device.ID),
		}
		if progress, found := pending[device.ID]; found {
			deviceSnapshot.Pending = &progress
		}

		latest.Devices = append(latest.Devices, deviceSnapshot)
	}

	published := latest.Copy()
	monitor.snapshot.Store(&published)
}
