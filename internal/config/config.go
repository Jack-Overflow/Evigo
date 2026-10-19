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
	Devices  []Device `yaml:"devices"`
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
	StablePowerDelay    time.Duration `yaml:"stable_power_delay"`
	ShutdownGracePeriod time.Duration `yaml:"shutdown_grace_period"`
	MinBatteryPercent   float64       `yaml:"min_battery_percent"`
	RetryInterval       time.Duration `yaml:"retry_interval"`
	DeviceTimeout       time.Duration `yaml:"device_timeout"`
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
		StablePowerDelay:    30 * time.Second,
		ShutdownGracePeriod: 2 * time.Minute,
		MinBatteryPercent:   25,
		RetryInterval:       30 * time.Second,
		DeviceTimeout:       10 * time.Minute,
	}
	settings.Probe = Probe{Timeout: 2 * time.Second}
	settings.WOL = WOL{Broadcast: "255.255.255.255", Port: 9}

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
	if settings.NUT.Password != "" && settings.NUT.PasswordFile != "" {
		return errors.New("nut.password and nut.password_file are mutually exclusive")
	}

	var err error
	if settings.NUT.Username, err = expandCredential(settings.NUT.Username, "nut.username"); err != nil {
		return err
	}

	if settings.NUT.Password, err = expandCredential(settings.NUT.Password, "nut.password"); err != nil {
		return err
	}

	if settings.NUT.PasswordFile != "" {
		settings.NUT.Password, err = readPasswordFile(settings.NUT.PasswordFile)
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

func readPasswordFile(path string) (string, error) {
	passwordFile, err := os.Open(path)
	if err != nil {
		return "", errors.New("cannot open nut.password_file")
	}

	defer passwordFile.Close()
	contents, err := io.ReadAll(io.LimitReader(passwordFile, 4097))
	if err != nil || len(contents) > 4096 {
		return "", errors.New("cannot read nut.password_file (maximum 4096 bytes)")
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
