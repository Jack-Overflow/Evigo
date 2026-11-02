package config

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
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
		settings.validateAlerts,
		settings.validateDiscord,
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

func (settings Config) validateAlerts() error {
	email := settings.Alerts.Email
	if email.Host == "" {
		if email.From != "" || len(email.To) != 0 || email.Username != "" || email.Password != "" {
			return errors.New("alerts.email.host is required to send email")
		}

		return nil
	}

	switch {
	case !hostOK(email.Host):
		return errors.New("alerts.email.host is invalid")
	case !portOK(email.Port):
		return errors.New("alerts.email.port must be between 1 and 65535")
	case !slices.Contains([]string{"starttls", "tls", "none"}, email.Security):
		return errors.New("alerts.email.security must be starttls, tls, or none")
	case !emailAddressOK(email.From):
		return errors.New("alerts.email.from must be a plain email address")
	case len(email.To) == 0 || !allEmailAddressesOK(email.To):
		return errors.New("alerts.email.to must list at least one plain email address")
	case (email.Username == "") != (email.Password == ""):
		return errors.New("email authentication requires both username and password")
	case email.Username != "" && (!textOK(email.Username) || !textOK(email.Password)):
		return errors.New("invalid email credentials: line breaks and NUL are not allowed")
	case email.Username != "" && email.Security == "none" && !slices.Contains([]string{"localhost", "127.0.0.1", "::1"}, email.Host):
		return errors.New("alerts.email.security none only allows a login to localhost")
	}

	return nil
}

func (settings Config) validateDiscord() error {
	webhookURL := settings.Alerts.Discord.WebhookURL
	if webhookURL == "" {
		return nil
	}

	parsed, err := url.Parse(webhookURL)
	if err != nil || parsed.Scheme != "https" || !slices.Contains([]string{"discord.com", "discordapp.com"}, parsed.Host) || !strings.HasPrefix(parsed.Path, "/api/webhooks/") {
		return errors.New("alerts.discord.webhook_url must be a https://discord.com/api/webhooks/ URL")
	}

	return nil
}

func emailAddressOK(address string) bool {
	parsed, err := mail.ParseAddress(address)

	return err == nil && parsed.Address == address
}

func allEmailAddressesOK(addresses []string) bool {
	for _, address := range addresses {
		if !emailAddressOK(address) {
			return false
		}
	}

	return true
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
