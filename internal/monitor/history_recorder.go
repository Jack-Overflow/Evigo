package monitor

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/devices"
	"github.com/Jack-Overflow/Evigo/internal/history"
	"github.com/Jack-Overflow/Evigo/internal/nut"
)

const (
	maxUnsavedEvents = 1000
	noReadingStatus  = "no reading"
)

type historyRecorder struct {
	storage          Storage
	log              *slog.Logger
	lastUPSStatus    string
	lastReachability map[string]devices.Reachability
	events           []history.Event
}

func (recorder *historyRecorder) record(events ...history.Event) {
	recorder.events = append(recorder.events, events...)
}

func (recorder *historyRecorder) recordUPSStatusIfChanged(at time.Time, reading *nut.Reading) bool {
	status := noReadingStatus
	var details history.Details
	if reading != nil {
		status = reading.LogStatus()
		details = history.Details{Status: status, BatteryPercent: &reading.Battery, OnBattery: reading.OnBattery()}
	}

	if status == recorder.lastUPSStatus {
		return false
	}

	recorder.lastUPSStatus = status
	recorder.record(history.Event{At: at, Kind: history.UPSStatus, Details: details})

	return true
}

func (recorder *historyRecorder) recordReachabilityChanges(at time.Time, configured []*devices.Device, reachability map[string]devices.Reachability) {
	for _, device := range configured {
		previous, seen := recorder.lastReachability[device.ID]
		current := reachability[device.ID]
		if seen && current == previous {
			continue
		}

		recorder.record(history.Event{
			At:       at,
			Kind:     history.Reachability,
			DeviceID: device.ID,
			Details:  history.Details{Reachability: current.String()},
		})
	}

	recorder.lastReachability = reachability
}

func (recorder *historyRecorder) save(ctx context.Context) {
	if len(recorder.events) == 0 {
		return
	}

	if err := recorder.storage.AppendHistory(ctx, recorder.events); err != nil {
		recorder.log.Error("cannot save history", "error", err)
		if overflow := len(recorder.events) - maxUnsavedEvents; overflow > 0 {
			recorder.events = recorder.events[overflow:]
		}

		return
	}

	recorder.events = nil
}
