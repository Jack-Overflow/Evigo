package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"evigo/internal/config"
	"evigo/internal/database"
	"evigo/internal/devices"
	"evigo/internal/monitor"
	"evigo/internal/nut"
	"evigo/internal/recovery"
)

var version = "dev"

var errUsage = errors.New("usage: evigo {run|validate|version} [--config path] [--verbose]")

func main() {
	if err := execute(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execute(arguments []string, stdout, stderr io.Writer) error {
	if len(arguments) == 0 {
		return errUsage
	}

	command, options := arguments[0], arguments[1:]
	switch command {
	case "version":
		if len(options) != 0 {
			return errUsage
		}

		fmt.Fprintln(stdout, "Evigo", version)

		return nil
	case "validate":
		settings, err := loadConfig(command, options, stderr)
		if err != nil {
			return err
		}

		if _, err = devices.FromConfig(settings); err != nil {
			return err
		}

		fmt.Fprintln(stdout, "Configuration valid")

		return nil
	case "run":
		settings, err := loadConfig(command, options, stderr)
		if err != nil {
			return err
		}

		return run(settings, stderr)
	}

	return errUsage
}

func loadConfig(command string, options []string, stderr io.Writer) (config.Config, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "config.yaml", "YAML configuration path")
	verbose := flags.Bool("verbose", false, "enable debug logging")
	if err := flags.Parse(options); err != nil {
		return config.Config{}, err
	}

	if flags.NArg() != 0 {
		return config.Config{}, errUsage
	}

	settings, err := config.Load(*configPath)
	if err != nil {
		return config.Config{}, err
	}

	if *verbose {
		settings.Log.Level = "debug"
	}

	return settings, nil
}

func run(settings config.Config, stderr io.Writer) error {
	level, _ := settings.Level()
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: level}))
	configuredDevices, err := devices.FromConfig(settings)
	if err != nil {
		return err
	}

	db, err := database.Open(settings.State.Path)
	if err != nil {
		return err
	}

	defer db.Close()
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	saved, err := db.LoadPending(ctx)
	if err != nil {
		return err
	}

	ups := nut.NewReader(nut.Settings{
		Host:           settings.NUT.Host,
		Port:           settings.NUT.Port,
		UPS:            settings.NUT.UPS,
		Username:       settings.NUT.Username,
		Password:       settings.NUT.Password,
		ConnectTimeout: settings.NUT.ConnectTimeout,
		ReadTimeout:    settings.NUT.ReadTimeout,
	}, nil)
	defer ups.Close()
	deviceRecovery := recovery.New(recovery.Settings{
		StablePowerDelay:    settings.Recovery.StablePowerDelay,
		ShutdownGracePeriod: settings.Recovery.ShutdownGracePeriod,
		MinBatteryPercent:   settings.Recovery.MinBatteryPercent,
		RetryInterval:       settings.Recovery.RetryInterval,
		DeviceTimeout:       settings.Recovery.DeviceTimeout,
		MaxReadingAge:       settings.NUT.MaxReadingAge,
	}, configuredDevices, saved, logger)
	loop := monitor.New(monitor.Settings{
		PollInterval:          settings.Poll.Interval,
		OnBatteryPollInterval: settings.Poll.OnBatteryInterval,
		PingTimeout:           settings.Probe.Timeout,
		MaxReadingAge:         settings.NUT.MaxReadingAge,
	}, ups, configuredDevices, deviceRecovery, db, saved, devices.RunCommand, logger)
	logger.Info("Evigo started", "version", version, "devices", len(configuredDevices), "pending", len(saved))

	return loop.Run(ctx)
}
