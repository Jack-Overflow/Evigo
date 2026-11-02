package alerts

import (
	"fmt"
	"time"
)

type Severity int

const (
	Problem Severity = iota
	Resolved
)

type Alert struct {
	At       time.Time
	Severity Severity
	Title    string
	Body     string
}

func OnBattery(at time.Time, batteryPercent float64) Alert {
	return Alert{
		At:       at,
		Severity: Problem,
		Title:    "UPS on battery",
		Body:     fmt.Sprintf("The UPS is running on battery at %.0f%%.", batteryPercent),
	}
}

func MainsRestored(at time.Time, batteryPercent float64) Alert {
	return Alert{
		At:       at,
		Severity: Resolved,
		Title:    "UPS mains power restored",
		Body:     fmt.Sprintf("Mains power is back and the battery is at %.0f%%.", batteryPercent),
	}
}

func DeviceRecovered(at time.Time, deviceName string, attempts int) Alert {
	return Alert{
		At:       at,
		Severity: Resolved,
		Title:    deviceName + " is back online",
		Body:     fmt.Sprintf("%s answered ping after %s.", deviceName, wakeAttempts(attempts)),
	}
}

func DeviceGaveUp(at time.Time, deviceName string, attempts int) Alert {
	body := fmt.Sprintf("%s did not answer ping after %s.", deviceName, wakeAttempts(attempts))
	if attempts == 0 {
		body = fmt.Sprintf("Evigo stopped waiting for %s. No wake was sent.", deviceName)
	}

	return Alert{At: at, Severity: Problem, Title: deviceName + " did not come back", Body: body}
}

func wakeAttempts(attempts int) string {
	if attempts == 1 {
		return "1 wake attempt"
	}

	return fmt.Sprintf("%d wake attempts", attempts)
}
