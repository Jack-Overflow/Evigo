package nut

import (
	"maps"
	"math"
	"slices"
	"strings"
	"time"
)

type Reading struct {
	At      time.Time
	Status  string
	Battery float64
	Values  map[string]string
}

var mainsFlags = []string{"OL", "CHRG", "TRIM", "BOOST"}

func (reading Reading) Copy() *Reading {
	values := make(map[string]string, len(reading.Values))
	maps.Copy(values, reading.Values)
	reading.Values = values

	return &reading
}

func (reading Reading) Flags() []string {
	return strings.Fields(reading.Status)
}

func (reading Reading) Charging() bool {
	return slices.Contains(reading.Flags(), "CHRG")
}

func (reading Reading) OnBattery() bool {
	flags := reading.Flags()

	return slices.Contains(flags, "OB") && !slices.Contains(flags, "OL")
}

func (reading Reading) Safe(minBatteryPercent float64) bool {
	flags := reading.Flags()
	if !slices.Contains(flags, "OL") || !validBattery(reading.Battery) || reading.Battery < minBatteryPercent {
		return false
	}

	for _, flag := range flags {
		if !slices.Contains(mainsFlags, flag) {
			return false
		}
	}

	return true
}

func validBattery(percent float64) bool {
	return !math.IsNaN(percent) && !math.IsInf(percent, 0) && percent >= 0 && percent <= 100
}

func (reading Reading) LogStatus() string {
	var flags []string
	for _, flag := range reading.Flags() {
		switch flag {
		case "OL", "OB", "CHRG", "DISCHRG", "LB", "RB", "FSD", "OFF", "BYPASS", "CAL", "OVER", "TRIM", "BOOST", "ALARM":
		default:
			flag = "UNKNOWN"
		}

		if !slices.Contains(flags, flag) {
			flags = append(flags, flag)
		}
	}

	return strings.Join(flags, " ")
}
