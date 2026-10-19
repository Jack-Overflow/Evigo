package history

import "time"

type Kind string

const (
	Started         Kind = "started"
	UPSStatus       Kind = "ups_status"
	Reachability    Kind = "reachability"
	DevicePending   Kind = "device_pending"
	WakeRequested   Kind = "wake_requested"
	WakeSent        Kind = "wake_sent"
	WakeFailed      Kind = "wake_failed"
	DeviceRecovered Kind = "device_recovered"
	DeviceGaveUp    Kind = "device_gave_up"
)

type Source string

const (
	Automatic Source = "automatic"
	Manual    Source = "manual"
)

type Event struct {
	ID       int64
	At       time.Time
	Kind     Kind
	DeviceID string
	Details  Details
}

type Details struct {
	Version        string   `json:"version,omitempty"`
	Status         string   `json:"status,omitempty"`
	BatteryPercent *float64 `json:"battery_percent,omitempty"`
	Reachability   string   `json:"reachability,omitempty"`
	Source         Source   `json:"source,omitempty"`
	Attempt        int      `json:"attempt,omitempty"`
}

type Page struct {
	Events     []Event
	NextBefore int64
}
