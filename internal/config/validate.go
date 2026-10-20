package config

import (
	"errors"
	"fmt"
	"math"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	idPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,252}[A-Za-z0-9])?$`)
)

func (settings Config) Validate() error {
	for _, validate := range []func() error{
		settings.validateBasics,
		settings.validateNUT,
		settings.validateDurations,
		settings.validateRecovery,
		settings.validateWeb,
		settings.validateDevices,
	} {
		if err := validate(); err != nil {
			return err
		}
	}

	return nil
}

func (settings Config) validateBasics() error {
	if _, err := settings.Level(); err != nil {
		return err
	}

	if !textOK(settings.State.Path) {
		return errors.New("state.path is required")
	}

	return validateWOL(settings.WOL)
}

func (settings Config) validateNUT() error {
	switch {
	case !hostOK(settings.NUT.Host) || !textOK(settings.NUT.UPS):
		return errors.New("nut.host and nut.ups are required")
	case !portOK(settings.NUT.Port):
		return errors.New("nut.port must be between 1 and 65535")
	case (settings.NUT.Username == "") != (settings.NUT.Password == ""):
		return errors.New("NUT authentication requires both username and password")
	case settings.NUT.Username != "" && (!textOK(settings.NUT.Username) || !textOK(settings.NUT.Password)):
		return errors.New("invalid NUT credentials: line breaks and NUL are not allowed")
	case settings.NUT.MaxReadingAge < settings.Poll.Interval+settings.NUT.ReadTimeout:
		return errors.New("nut.max_reading_age must cover poll.interval plus nut.read_timeout")
	}

	return nil
}

func (settings Config) validateDurations() error {
	for _, setting := range []struct {
		name  string
		value time.Duration
	}{
		{"nut.connect_timeout", settings.NUT.ConnectTimeout},
		{"nut.read_timeout", settings.NUT.ReadTimeout},
		{"nut.max_reading_age", settings.NUT.MaxReadingAge},
		{"poll.interval", settings.Poll.Interval},
		{"poll.on_battery_interval", settings.Poll.OnBatteryInterval},
		{"recovery.stable_power_delay", settings.Recovery.StablePowerDelay},
		{"recovery.retry_interval", settings.Recovery.RetryInterval},
		{"recovery.device_timeout", settings.Recovery.DeviceTimeout},
		{"probe.timeout", settings.Probe.Timeout},
	} {
		if setting.value <= 0 || setting.value > 7*24*time.Hour {
			return fmt.Errorf("%s must be positive and at most 168h", setting.name)
		}
	}

	return nil
}

func (settings Config) validateRecovery() error {
	percent := settings.Recovery.MinBatteryPercent
	if math.IsNaN(percent) || percent < 0 || percent > 100 {
		return errors.New("recovery.min_battery_percent must be between 0 and 100")
	}

	return nil
}

func (settings Config) validateWeb() error {
	if settings.Web.Listen == "" {
		return nil
	}

	host, port, err := net.SplitHostPort(settings.Web.Listen)
	if err != nil || !numericPortOK(port) || (host != "" && net.ParseIP(host) == nil) {
		return errors.New("web.listen must be an IP address and port, such as 127.0.0.1:8080")
	}

	for _, allowed := range settings.Web.AllowedHosts {
		if !allowedHostOK(allowed) {
			return errors.New("web.allowed_hosts entries must be a hostname or IP address, without a scheme, port or path")
		}
	}

	return nil
}

func allowedHostOK(allowed string) bool {
	return net.ParseIP(allowed) != nil || hostnamePattern.MatchString(allowed)
}

func numericPortOK(port string) bool {
	number, err := strconv.Atoi(port)

	return err == nil && portOK(number)
}

func (settings Config) validateDevices() error {
	if len(settings.Devices) == 0 {
		return errors.New("configure at least one device")
	}

	seen := map[string]bool{}
	for _, configuredDevice := range settings.Devices {
		if !idPattern.MatchString(configuredDevice.ID) || seen[configuredDevice.ID] {
			return errors.New("device IDs must be unique and contain only letters, digits, dot, dash, or underscore")
		}

		seen[configuredDevice.ID] = true
		if err := configuredDevice.validate(); err != nil {
			return fmt.Errorf("device %s: %w", configuredDevice.ID, err)
		}
	}

	return nil
}

func (configuredDevice Device) validate() error {
	if !textOK(configuredDevice.Name) || !hostOK(configuredDevice.Host) {
		return errors.New("requires a name and host")
	}

	if configuredDevice.MAC == "" {
		return errors.New("requires a mac address")
	}

	if configuredDevice.WOL.Broadcast != "" && net.ParseIP(configuredDevice.WOL.Broadcast).To4() == nil {
		return errors.New("wol.broadcast must be an IPv4 address")
	}

	if configuredDevice.WOL.Port != 0 && !portOK(configuredDevice.WOL.Port) {
		return errors.New("wol.port must be between 1 and 65535")
	}

	return nil
}

func validateWOL(destination WOL) error {
	if net.ParseIP(destination.Broadcast).To4() == nil || !portOK(destination.Port) {
		return errors.New("wol.broadcast must be an IPv4 address and wol.port between 1 and 65535")
	}

	return nil
}

func textOK(value string) bool {
	return value != "" && len(value) <= 4096 && !strings.ContainsAny(value, "\x00\r\n")
}

func hostOK(host string) bool {
	return textOK(host) && !strings.ContainsAny(host, " \t/@") && !strings.HasPrefix(host, "-")
}

func portOK(port int) bool {
	return port > 0 && port <= 65535
}
