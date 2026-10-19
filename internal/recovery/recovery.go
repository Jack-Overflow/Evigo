package recovery

import (
	"log/slog"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/devices"
	"github.com/Jack-Overflow/Evigo/internal/nut"
)

type Block string

const (
	NoBlock      Block = ""
	NoReading    Block = "no_reading"
	OnBattery    Block = "on_battery"
	BatteryLow   Block = "battery_low"
	UnsafeStatus Block = "unsafe_status"
	Stabilising  Block = "stabilising"
)

type Progress struct {
	Attempts    int
	LastAttempt time.Time
	TimeLeft    time.Duration
	GraceLeft   time.Duration
}

type Pass struct {
	At           time.Time
	Reading      *nut.Reading
	Reachability map[string]devices.Reachability
}

type Settings struct {
	StablePowerDelay    time.Duration
	ShutdownGracePeriod time.Duration
	MinBatteryPercent   float64
	RetryInterval       time.Duration
	DeviceTimeout       time.Duration
	MaxReadingAge       time.Duration
}

type Recovery struct {
	settings               Settings
	devices                []*devices.Device
	pending                map[string]*Progress
	wakeSent               map[string]bool
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
		wakeSent: map[string]bool{},
		log:      log,
	}
	for _, device := range configured {
		if progress, found := saved[device.ID]; found {
			recovery.pending[device.ID] = &progress
		}
	}

	return recovery
}

func (recovery *Recovery) Update(pass Pass) []*devices.Device {
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
	recovery.trackOutage(pass)
	stable := recovery.powerStable(pass.At)
	if stable && recovery.wasStable && elapsed > 0 {
		recovery.spendRecoveryTime(elapsed)
	}

	recovery.wasStable = stable
	if !stable {
		return nil
	}

	return recovery.updatePendingDevices(pass)
}

func (recovery *Recovery) WakeSent(deviceID string) {
	if progress := recovery.pending[deviceID]; progress != nil {
		recovery.wakeSent[deviceID] = true
	}
}

func (settings Settings) PowerBlock(at time.Time, reading *nut.Reading, safeSince time.Time) Block {
	switch {
	case reading == nil || at.Before(reading.At) || at.Sub(reading.At) >= settings.MaxReadingAge:
		return NoReading
	case reading.OnBattery():
		return OnBattery
	case reading.Safe(0) && !reading.Safe(settings.MinBatteryPercent):
		return BatteryLow
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

func (recovery *Recovery) SafeSince() time.Time {
	return recovery.safeSince
}

func (recovery *Recovery) WakeSentTo(deviceID string) bool {
	return recovery.wakeSent[deviceID]
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

func (recovery *Recovery) trackOutage(pass Pass) {
	switch {
	case pass.Reading != nil && pass.Reading.Safe(0):
		recovery.devicesOnline = onlineDevices(pass.Reachability)
		recovery.devicesOnlineCheckedAt = pass.At
	case pass.Reading != nil && pass.Reading.OnBattery():
		recovery.addOutageDevicesToPending(pass.At)
	}
}

func (recovery *Recovery) addOutageDevicesToPending(at time.Time) {
	for deviceID, progress := range recovery.pending {
		progress.GraceLeft = recovery.settings.ShutdownGracePeriod
		delete(recovery.wakeSent, deviceID)
	}

	if at.Sub(recovery.devicesOnlineCheckedAt) > recovery.settings.MaxReadingAge {
		return
	}

	for _, device := range recovery.devices {
		if recovery.devicesOnline[device.ID] && recovery.pending[device.ID] == nil {
			recovery.pending[device.ID] = &Progress{
				TimeLeft:  recovery.settings.DeviceTimeout,
				GraceLeft: recovery.settings.ShutdownGracePeriod,
			}
			recovery.log.Info("device will be woken after the outage", "device", device.ID)
		}
	}
}

func (recovery *Recovery) powerStable(at time.Time) bool {
	return !recovery.safeSince.IsZero() && at.Sub(recovery.safeSince) >= recovery.settings.StablePowerDelay
}

func (recovery *Recovery) spendRecoveryTime(elapsed time.Duration) {
	for deviceID, progress := range recovery.pending {
		timeoutElapsed := elapsed
		if !recovery.wakeSent[deviceID] {
			timeoutElapsed = max(0, elapsed-progress.GraceLeft)
		}

		progress.GraceLeft = max(0, progress.GraceLeft-elapsed)
		progress.TimeLeft = max(0, progress.TimeLeft-timeoutElapsed)
	}
}

func (recovery *Recovery) updatePendingDevices(pass Pass) []*devices.Device {
	var due []*devices.Device
	for _, device := range recovery.devices {
		progress := recovery.pending[device.ID]
		if progress == nil {
			continue
		}

		switch reachability := pass.Reachability[device.ID]; {
		case reachability == devices.Online && (recovery.wakeSent[device.ID] || progress.GraceLeft == 0):
			delete(recovery.pending, device.ID)
			delete(recovery.wakeSent, device.ID)
			recovery.log.Info("device is back online", "device", device.ID, "attempts", progress.Attempts)
		case progress.TimeLeft == 0:
			delete(recovery.pending, device.ID)
			delete(recovery.wakeSent, device.ID)
			recovery.log.Warn("device did not come back in time", "device", device.ID, "attempts", progress.Attempts)
		case reachability == devices.Offline && recovery.retryDue(progress, pass.At):
			progress.Attempts++
			progress.LastAttempt = pass.At
			due = append(due, device)
		}
	}

	return due
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
