package history

import "time"

type Kind string

const (
	Started         Kind = "started"
	UPSStatus       Kind = "ups_status"
	Reachability    Kind = "reachability"
	DeviceWatched   Kind = "device_watched"
	WakeSent        Kind = "wake_sent"
	WakeFailed      Kind = "wake_failed"
	DeviceRecovered Kind = "device_recovered"
	DeviceAlreadyOn Kind = "device_already_on"
	DeviceGaveUp    Kind = "device_gave_up"
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
	OnBattery      bool     `json:"on_battery,omitempty"`
	Attempt        int      `json:"attempt,omitempty"`
}

type Page struct {
	Events     []Event
	NextBefore int64
}
