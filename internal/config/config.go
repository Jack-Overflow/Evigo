package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	Log struct {
		Level string `yaml:"level"`
	} `yaml:"log"`
	State struct {
		Path string `yaml:"path"`
	} `yaml:"state"`
	NUT      NUT      `yaml:"nut"`
	Poll     Poll     `yaml:"poll"`
	Recovery Recovery `yaml:"recovery"`
	Probe    Probe    `yaml:"probe"`
	WOL      WOL      `yaml:"wol"`
	Web      Web      `yaml:"web"`
	Alerts   Alerts   `yaml:"alerts"`
	Devices  []Device `yaml:"devices"`
}

type Alerts struct {
	Email   Email   `yaml:"email"`
	Discord Discord `yaml:"discord"`
}

type Discord struct {
	WebhookURL string `yaml:"webhook_url"`
}

type Email struct {
	Host         string   `yaml:"host"`
	Port         int      `yaml:"port"`
	Security     string   `yaml:"security"`
	Username     string   `yaml:"username"`
	Password     string   `yaml:"password"`
	PasswordFile string   `yaml:"password_file"`
	From         string   `yaml:"from"`
	To           []string `yaml:"to"`
}

type Web struct {
	Listen       string   `yaml:"listen"`
	AllowedHosts []string `yaml:"allowed_hosts"`
}

type NUT struct {
	Host           string        `yaml:"host"`
	Port           int           `yaml:"port"`
	UPS            string        `yaml:"ups"`
	Username       string        `yaml:"username"`
	Password       string        `yaml:"password"`
	PasswordFile   string        `yaml:"password_file"`
	ConnectTimeout time.Duration `yaml:"connect_timeout"`
	ReadTimeout    time.Duration `yaml:"read_timeout"`
	MaxReadingAge  time.Duration `yaml:"max_reading_age"`
}

type Poll struct {
	Interval          time.Duration `yaml:"interval"`
	OnBatteryInterval time.Duration `yaml:"on_battery_interval"`
}

type Recovery struct {
	StablePowerDelay  time.Duration `yaml:"stable_power_delay"`
	MinBatteryPercent float64       `yaml:"min_battery_percent"`
	RetryInterval     time.Duration `yaml:"retry_interval"`
	DeviceTimeout     time.Duration `yaml:"device_timeout"`
}

type Probe struct {
	Timeout time.Duration `yaml:"timeout"`
}

type WOL struct {
	Broadcast string `yaml:"broadcast"`
	Port      int    `yaml:"port"`
}

type Device struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	MAC  string `yaml:"mac"`
	WOL  WOL    `yaml:"wol"`
}

var credentialReference = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func Defaults() Config {
	var settings Config
	settings.Log.Level = "info"
	settings.State.Path = "./evigo.db"
	settings.NUT = NUT{Port: 3493, ConnectTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, MaxReadingAge: 30 * time.Second}
	settings.Poll = Poll{Interval: 10 * time.Second, OnBatteryInterval: 2 * time.Second}
	settings.Recovery = Recovery{
		StablePowerDelay:  30 * time.Second,
		MinBatteryPercent: 25,
		RetryInterval:     30 * time.Second,
		DeviceTimeout:     10 * time.Minute,
	}
	settings.Probe = Probe{Timeout: 2 * time.Second}
	settings.WOL = WOL{Broadcast: "255.255.255.255", Port: 9}
	settings.Alerts.Email = Email{Port: 587, Security: "starttls"}

	return settings
}

func Load(path string) (Config, error) {
	settings := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		return settings, fmt.Errorf("read configuration: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err = decoder.Decode(&document); err == io.EOF {
		return settings, errors.New("configuration is empty")
	} else if err != nil {
		// Parser errors can quote values, including credentials.
		return settings, errors.New("invalid YAML syntax in configuration")
	}

	var extra yaml.Node
	if decoder.Decode(&extra) != io.EOF {
		return settings, errors.New("configuration must contain exactly one YAML document")
	}

	if err = checkFields(document.Content[0], reflect.TypeFor[Config](), ""); err != nil {
		return settings, err
	}

	if err = document.Decode(&settings); err != nil {
		return settings, errors.New("invalid YAML configuration")
	}

	if err = settings.resolveCredentials(); err != nil {
		return settings, err
	}

	return settings, settings.Validate()
}

func (settings *Config) resolveCredentials() error {
	if err := resolveLogin("nut", &settings.NUT.Username, &settings.NUT.Password, settings.NUT.PasswordFile); err != nil {
		return err
	}

	email := &settings.Alerts.Email
	if err := resolveLogin("alerts.email", &email.Username, &email.Password, email.PasswordFile); err != nil {
		return err
	}

	discord := &settings.Alerts.Discord
	var err error
	discord.WebhookURL, err = expandCredential(discord.WebhookURL, "alerts.discord.webhook_url")

	return err
}

func resolveLogin(section string, username, password *string, passwordFile string) error {
	if *password != "" && passwordFile != "" {
		return fmt.Errorf("%s.password and %s.password_file are mutually exclusive", section, section)
	}

	var err error
	if *username, err = expandCredential(*username, section+".username"); err != nil {
		return err
	}

	if *password, err = expandCredential(*password, section+".password"); err != nil {
		return err
	}

	if passwordFile != "" {
		*password, err = readPasswordFile(passwordFile, section+".password_file")
	}

	return err
}

func expandCredential(value, field string) (string, error) {
	missing := false
	expanded := credentialReference.ReplaceAllStringFunc(value, func(reference string) string {
		environmentValue := os.Getenv(credentialReference.FindStringSubmatch(reference)[1])
		missing = missing || environmentValue == ""

		return environmentValue
	})
	if missing || strings.Contains(expanded, "${") {
		return "", fmt.Errorf("%s: unresolved credential reference", field)
	}

	return expanded, nil
}

func readPasswordFile(path, field string) (string, error) {
	passwordFile, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open %s", field)
	}

	defer passwordFile.Close()
	contents, err := io.ReadAll(io.LimitReader(passwordFile, 4097))
	if err != nil || len(contents) > 4096 {
		return "", fmt.Errorf("cannot read %s (maximum 4096 bytes)", field)
	}

	return strings.TrimSuffix(strings.TrimSuffix(string(contents), "\n"), "\r"), nil
}

func (settings Config) Level() (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(settings.Log.Level)); err != nil {
		return level, errors.New("log.level must be debug, info, warn, or error")
	}

	return level, nil
}
