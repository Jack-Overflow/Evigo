package recovery

import (
	"log/slog"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/devices"
	"github.com/Jack-Overflow/Evigo/internal/history"
	"github.com/Jack-Overflow/Evigo/internal/nut"
)

type Block string

const (
	NoBlock      Block = ""
	NoReading    Block = "no_reading"
	OnBattery    Block = "on_battery"
	BatteryLow   Block = "battery_low"
	UnsafeStatus Block = "unsafe_status"
	Settling     Block = "settling"
	Stabilising  Block = "stabilising"
)

type State string

const (
	Watching State = "watching"
	Waking   State = "waking"
)

type Progress struct {
	Attempts    int
	LastAttempt time.Time
	TimeLeft    time.Duration
}

func (progress Progress) State(reachability devices.Reachability) State {
	if reachability == devices.Offline || progress.Attempts > 0 {
		return Waking
	}

	return Watching
}

type Outcome struct {
	Wake   []*devices.Device
	Events []history.Event
}

type Pass struct {
	At           time.Time
	Reading      *nut.Reading
	Reachability map[string]devices.Reachability
}

type Settings struct {
	StablePowerDelay  time.Duration
	MinBatteryPercent float64
	RetryInterval     time.Duration
	DeviceTimeout     time.Duration
	MaxReadingAge     time.Duration
}

type Recovery struct {
	settings               Settings
	devices                []*devices.Device
	pending                map[string]*Progress
	safeSince              time.Time
	wasStable              bool
	previousPass           time.Time
	devicesOnline          map[string]bool
	devicesOnlineCheckedAt time.Time
	log                    *slog.Logger
}

func New(settings Settings, configured []*devices.Device, saved map[string]Progress, log *slog.Logger) *Recovery {
	recovery := &Recovery{
		settings: settings,
		devices:  configured,
		pending:  map[string]*Progress{},
		log:      log,
	}
	for _, device := range configured {
		if progress, found := saved[device.ID]; found {
			recovery.pending[device.ID] = &progress
		}
	}

	return recovery
}

func (recovery *Recovery) Update(pass Pass) Outcome {
	elapsed := pass.At.Sub(recovery.previousPass)
	if !recovery.previousPass.IsZero() && (elapsed < 0 || elapsed > recovery.settings.MaxReadingAge) {
		recovery.safeSince = time.Time{}
		recovery.wasStable = false
		recovery.devicesOnline = nil
	}

	recovery.previousPass = pass.At
	if pass.Reading != nil && (pass.At.Before(pass.Reading.At) || pass.At.Sub(pass.Reading.At) > recovery.settings.MaxReadingAge) {
		pass.Reading = nil
	}

	recovery.trackSafePower(pass)
	outageEvents := recovery.trackOutage(pass)
	stable := recovery.powerStable(pass.At)
	if stable && recovery.wasStable && elapsed > 0 {
		recovery.spendRecoveryTime(elapsed)
	}

	recovery.wasStable = stable
	if !stable {
		return Outcome{Events: outageEvents}
	}

	due, deviceEvents := recovery.updatePendingDevices(pass)

	return Outcome{Wake: due, Events: append(outageEvents, deviceEvents...)}
}

func (settings Settings) PowerBlock(at time.Time, reading *nut.Reading, safeSince time.Time) Block {
	switch {
	case reading == nil || at.Before(reading.At) || at.Sub(reading.At) >= settings.MaxReadingAge:
		return NoReading
	case reading.OnBattery():
		return OnBattery
	case reading.Safe(0) && !reading.Safe(settings.MinBatteryPercent):
		return BatteryLow
	case !reading.Safe(0) && reading.Settling():
		return Settling
	case !reading.Safe(settings.MinBatteryPercent):
		return UnsafeStatus
	case safeSince.IsZero() || at.Sub(safeSince) < settings.StablePowerDelay:
		return Stabilising
	}

	return NoBlock
}

func (recovery *Recovery) Settings() Settings {
	return recovery.settings
}

func (recovery *Recovery) SpendingTimeLeft() bool {
	return recovery.wasStable
}

func (recovery *Recovery) SafeSince() time.Time {
	return recovery.safeSince
}

func (recovery *Recovery) StorageFailed() {
	recovery.safeSince = time.Time{}
	recovery.wasStable = false
}

func (recovery *Recovery) Pending() map[string]Progress {
	pending := make(map[string]Progress, len(recovery.pending))
	for deviceID, progress := range recovery.pending {
		pending[deviceID] = *progress
	}

	return pending
}

func (recovery *Recovery) trackSafePower(pass Pass) {
	if pass.Reading == nil || !pass.Reading.Safe(recovery.settings.MinBatteryPercent) {
		recovery.safeSince = time.Time{}
	} else if recovery.safeSince.IsZero() {
		recovery.safeSince = pass.At
	}
}

func (recovery *Recovery) trackOutage(pass Pass) []history.Event {
	switch {
	case pass.Reading != nil && pass.Reading.Safe(0):
		recovery.devicesOnline = onlineDevices(pass.Reachability)
		recovery.devicesOnlineCheckedAt = pass.At
	case pass.Reading != nil && pass.Reading.OnBattery():
		return recovery.addOutageDevicesToPending(pass.At)
	}

	return nil
}

func (recovery *Recovery) addOutageDevicesToPending(at time.Time) []history.Event {
	if at.Sub(recovery.devicesOnlineCheckedAt) > recovery.settings.MaxReadingAge {
		return nil
	}

	var events []history.Event
	for _, device := range recovery.devices {
		if recovery.devicesOnline[device.ID] && recovery.pending[device.ID] == nil {
			recovery.pending[device.ID] = &Progress{TimeLeft: recovery.settings.DeviceTimeout}
			recovery.log.Info("device will be woken after the outage", "device", device.ID)
			events = append(events, history.Event{At: at, Kind: history.DeviceWatched, DeviceID: device.ID})
		}
	}

	return events
}

func (recovery *Recovery) powerStable(at time.Time) bool {
	return !recovery.safeSince.IsZero() && at.Sub(recovery.safeSince) >= recovery.settings.StablePowerDelay
}

func (recovery *Recovery) spendRecoveryTime(elapsed time.Duration) {
	for _, progress := range recovery.pending {
		progress.TimeLeft = max(0, progress.TimeLeft-elapsed)
	}
}

func (recovery *Recovery) updatePendingDevices(pass Pass) ([]*devices.Device, []history.Event) {
	var due []*devices.Device
	var events []history.Event
	for _, device := range recovery.devices {
		progress := recovery.pending[device.ID]
		if progress == nil {
			continue
		}

		switch reachability := pass.Reachability[device.ID]; {
		case reachability == devices.Online && progress.Attempts == 0:
			delete(recovery.pending, device.ID)
			recovery.log.Info("device was already on", "device", device.ID)
			events = append(events, history.Event{At: pass.At, Kind: history.DeviceAlreadyOn, DeviceID: device.ID})
		case reachability == devices.Online:
			delete(recovery.pending, device.ID)
			recovery.log.Info("device is back online", "device", device.ID, "attempts", progress.Attempts)
			events = append(events, history.Event{
				At: pass.At, Kind: history.DeviceRecovered, DeviceID: device.ID, Details: history.Details{Attempt: progress.Attempts},
			})
		case progress.TimeLeft == 0:
			delete(recovery.pending, device.ID)
			recovery.log.Warn("device did not come back in time", "device", device.ID, "attempts", progress.Attempts)
			events = append(events, history.Event{
				At: pass.At, Kind: history.DeviceGaveUp, DeviceID: device.ID, Details: history.Details{Attempt: progress.Attempts},
			})
		case reachability == devices.Offline && recovery.retryDue(progress, pass.At):
			progress.Attempts++
			progress.LastAttempt = pass.At
			due = append(due, device)
		}
	}

	return due, events
}

func (recovery *Recovery) retryDue(progress *Progress, at time.Time) bool {
	return progress.LastAttempt.IsZero() ||
		at.Before(progress.LastAttempt) ||
		at.Sub(progress.LastAttempt) >= recovery.settings.RetryInterval
}

func onlineDevices(reachability map[string]devices.Reachability) map[string]bool {
	online := map[string]bool{}
	for deviceID, state := range reachability {
		if state == devices.Online {
			online[deviceID] = true
		}
	}

	return online
}
