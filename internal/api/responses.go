package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/history"
	"github.com/Jack-Overflow/Evigo/internal/recovery"
	"github.com/Jack-Overflow/Evigo/internal/snapshot"
)

type statusResponse struct {
	Version          string           `json:"version"`
	ServerTime       time.Time        `json:"server_time"`
	Monitor          monitorState     `json:"monitor"`
	CheckedAt        *time.Time       `json:"checked_at"`
	Power            powerResponse    `json:"power"`
	StorageAvailable bool             `json:"storage_available"`
	Settings         settingsResponse `json:"settings"`
	Devices          []deviceResponse `json:"devices"`
}

type powerResponse struct {
	UPS            string     `json:"ups"`
	Available      bool       `json:"available"`
	Status         []string   `json:"status"`
	BatteryPercent *float64   `json:"battery_percent"`
	ReadAt         *time.Time `json:"read_at"`
	OnBattery      bool       `json:"on_battery"`
	Charging       bool       `json:"charging"`
	Block          *string    `json:"block"`
	StableSince    *time.Time `json:"stable_since"`
	WakeAllowedAt  *time.Time `json:"wake_allowed_at"`
}

type settingsResponse struct {
	MinBatteryPercent    float64 `json:"min_battery_percent"`
	StablePowerSeconds   int     `json:"stable_power_seconds"`
	RetrySeconds         int     `json:"retry_seconds"`
	DeviceTimeoutSeconds int     `json:"device_timeout_seconds"`
}

type deviceResponse struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	Host         string                  `json:"host"`
	MAC          string                  `json:"mac"`
	Reachability string                  `json:"reachability"`
	Recovery     *deviceRecoveryResponse `json:"recovery"`
}

type deviceRecoveryResponse struct {
	State           recovery.State `json:"state"`
	Attempts        int            `json:"attempts"`
	LastAttemptAt   *time.Time     `json:"last_attempt_at"`
	TimeLeftSeconds int            `json:"time_left_seconds"`
	GivesUpAt       *time.Time     `json:"gives_up_at"`
}

func (server *Server) statusResponse(latest snapshot.Snapshot) statusResponse {
	response := statusResponse{
		Version:          server.settings.Version,
		ServerTime:       server.now(),
		Monitor:          server.monitorState(latest),
		CheckedAt:        timeOrNil(latest.CheckedAt),
		Power:            server.powerResponse(latest),
		StorageAvailable: latest.StorageOK,
		Settings: settingsResponse{
			MinBatteryPercent:    latest.RecoverySettings.MinBatteryPercent,
			StablePowerSeconds:   seconds(latest.RecoverySettings.StablePowerDelay),
			RetrySeconds:         seconds(latest.RecoverySettings.RetryInterval),
			DeviceTimeoutSeconds: seconds(latest.RecoverySettings.DeviceTimeout),
		},
		Devices: []deviceResponse{},
	}
	spendingTimeLeft := latest.SpendingTimeLeft && latest.PowerBlock(server.now()) == recovery.NoBlock
	for _, device := range latest.Devices {
		response.Devices = append(response.Devices, deviceResponseFor(device, latest.CheckedAt, spendingTimeLeft))
	}

	return response
}

func (server *Server) powerResponse(latest snapshot.Snapshot) powerResponse {
	power := powerResponse{UPS: server.settings.UPS, Status: []string{}}
	block := latest.PowerBlock(server.now())
	if block != recovery.NoBlock {
		power.Block = new(string(block))
	}

	reading := latest.Reading
	if reading == nil {
		return power
	}

	power.Available = block != recovery.NoReading
	power.Status = strings.Fields(reading.LogStatus())
	power.BatteryPercent = new(reading.Battery)
	power.ReadAt = timeOrNil(reading.At)
	power.OnBattery = reading.OnBattery()
	power.Charging = reading.Charging()
	if power.Available && !latest.SafeSince.IsZero() {
		power.StableSince = timeOrNil(latest.SafeSince)
		power.WakeAllowedAt = timeOrNil(latest.SafeSince.Add(latest.RecoverySettings.StablePowerDelay))
	}

	return power
}

func deviceResponseFor(device snapshot.Device, checkedAt time.Time, spendingTimeLeft bool) deviceResponse {
	response := deviceResponse{
		ID:           device.Device.ID,
		Name:         device.Device.Name,
		Host:         device.Device.Host,
		MAC:          device.Device.MAC.String(),
		Reachability: device.Reachability.String(),
	}
	if progress := device.Pending; progress != nil {
		response.Recovery = &deviceRecoveryResponse{
			State:           progress.State(device.Reachability),
			Attempts:        progress.Attempts,
			LastAttemptAt:   timeOrNil(progress.LastAttempt),
			TimeLeftSeconds: seconds(progress.TimeLeft),
		}
		if spendingTimeLeft {
			response.Recovery.GivesUpAt = timeOrNil(checkedAt.Add(progress.TimeLeft))
		}
	}

	return response
}

type historyPageResponse struct {
	Events     []historyEventResponse `json:"events"`
	NextBefore *int64                 `json:"next_before"`
}

type historyEventResponse struct {
	ID       int64           `json:"id"`
	At       time.Time       `json:"at"`
	Kind     history.Kind    `json:"kind"`
	DeviceID string          `json:"device_id,omitempty"`
	Details  history.Details `json:"details"`
}

func historyResponse(page history.Page) historyPageResponse {
	response := historyPageResponse{Events: []historyEventResponse{}}
	for _, event := range page.Events {
		response.Events = append(response.Events, historyEventResponse{
			ID:       event.ID,
			At:       event.At,
			Kind:     event.Kind,
			DeviceID: event.DeviceID,
			Details:  event.Details,
		})
	}

	if page.NextBefore != 0 {
		response.NextBefore = &page.NextBefore
	}

	return response
}

func timeOrNil(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}

	return &value
}

func seconds(duration time.Duration) int {
	return int(duration.Round(time.Second) / time.Second)
}

func writeJSON(writer http.ResponseWriter, statusCode int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(statusCode)
	json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, statusCode int, code string) {
	writeJSON(writer, statusCode, map[string]string{"error": code})
}
