package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/alerts"
	"github.com/Jack-Overflow/Evigo/internal/alerts/discord"
	"github.com/Jack-Overflow/Evigo/internal/alerts/email"
	"github.com/Jack-Overflow/Evigo/internal/alerts/ntfy"
	"github.com/Jack-Overflow/Evigo/internal/alerts/webhook"
	"github.com/Jack-Overflow/Evigo/internal/api"
	"github.com/Jack-Overflow/Evigo/internal/config"
	"github.com/Jack-Overflow/Evigo/internal/database"
	"github.com/Jack-Overflow/Evigo/internal/devices"
	"github.com/Jack-Overflow/Evigo/internal/history"
	"github.com/Jack-Overflow/Evigo/internal/monitor"
	"github.com/Jack-Overflow/Evigo/internal/nut"
	"github.com/Jack-Overflow/Evigo/internal/recovery"
)

var version = "dev"

var errUsage = errors.New("usage: evigo {run|validate|test-alerts|version} [--config path] [--verbose]")

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
	case "test-alerts":
		settings, err := loadConfig(command, options, stderr)
		if err != nil {
			return err
		}

		return testAlerts(alertSenders(settings), stdout)
	}

	return errUsage
}

func testAlerts(senders map[string]alerts.Sender, stdout io.Writer) error {
	if len(senders) == 0 {
		return errors.New("no alert providers are configured")
	}

	alert := alerts.Test(time.Now())
	failed := 0
	for _, provider := range slices.Sorted(maps.Keys(senders)) {
		ctx, cancel := context.WithTimeout(context.Background(), alerts.SendTimeout)
		err := senders[provider].Send(ctx, alert)
		cancel()
		if err != nil {
			failed++
			fmt.Fprintf(stdout, "%-8s failed: %v\n", provider, err)

			continue
		}

		fmt.Fprintf(stdout, "%-8s sent\n", provider)
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d alert providers failed", failed, len(senders))
	}

	return nil
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

	ups := nut.NewReader(nutSettings(settings), nil)
	defer ups.Close()
	deviceRecovery := recovery.New(recoverySettings(settings), configuredDevices, saved, logger)
	alertService := alerts.NewService(alertSenders(settings), logger)
	loop := monitor.New(monitorSettings(settings), ups, configuredDevices, deviceRecovery, db, saved, devices.RunCommand, alertService, logger)
	services := []func(context.Context) error{loop.Run}
	if settings.Web.Listen != "" {
		listener, err := net.Listen("tcp", settings.Web.Listen)
		if err != nil {
			return fmt.Errorf("cannot start web listener: %w", err)
		}

		defer listener.Close()
		server := api.New(webSettings(settings), loop.Snapshot, db.History)
		services = append(services, func(ctx context.Context) error { return server.Serve(ctx, listener) })
	}

	recordStarted(ctx, db, logger)
	logger.Info("Evigo started", "version", version, "devices", len(configuredDevices), "pending", len(saved), "web", settings.Web.Listen)

	return runAllUntilOneStops(ctx, services)
}

func recordStarted(ctx context.Context, db *database.DB, logger *slog.Logger) {
	started := history.Event{At: time.Now(), Kind: history.Started, Details: history.Details{Version: version}}
	if err := db.AppendHistory(ctx, []history.Event{started}); err != nil {
		logger.Error("cannot save history", "error", err)
	}
}

func alertSenders(settings config.Config) map[string]alerts.Sender {
	senders := map[string]alerts.Sender{}
	if configured := settings.Alerts.Email; configured.Host != "" {
		senders["email"] = email.New(email.Settings{
			Host:     configured.Host,
			Port:     configured.Port,
			Security: email.Security(configured.Security),
			Username: configured.Username,
			Password: configured.Password,
			From:     configured.From,
			To:       configured.To,
		})
	}

	if webhookURL := settings.Alerts.Discord.WebhookURL; webhookURL != "" {
		senders["discord"] = discord.New(webhookURL)
	}

	if configured := settings.Alerts.Ntfy; configured.Topic != "" {
		senders["ntfy"] = ntfy.New(ntfy.Settings{URL: configured.URL, Topic: configured.Topic, Token: configured.Token})
	}

	if url := settings.Alerts.Webhook.URL; url != "" {
		senders["webhook"] = webhook.New(url)
	}

	return senders
}

func nutSettings(settings config.Config) nut.Settings {
	return nut.Settings{
		Host:           settings.NUT.Host,
		Port:           settings.NUT.Port,
		UPS:            settings.NUT.UPS,
		Username:       settings.NUT.Username,
		Password:       settings.NUT.Password,
		ConnectTimeout: settings.NUT.ConnectTimeout,
		ReadTimeout:    settings.NUT.ReadTimeout,
	}
}

func recoverySettings(settings config.Config) recovery.Settings {
	return recovery.Settings{
		StablePowerDelay:  settings.Recovery.StablePowerDelay,
		MinBatteryPercent: settings.Recovery.MinBatteryPercent,
		RetryInterval:     settings.Recovery.RetryInterval,
		DeviceTimeout:     settings.Recovery.DeviceTimeout,
		MaxReadingAge:     settings.NUT.MaxReadingAge,
	}
}

func monitorSettings(settings config.Config) monitor.Settings {
	return monitor.Settings{
		PollInterval:          settings.Poll.Interval,
		OnBatteryPollInterval: settings.Poll.OnBatteryInterval,
		PingTimeout:           settings.Probe.Timeout,
		MaxReadingAge:         settings.NUT.MaxReadingAge,
	}
}

func webSettings(settings config.Config) api.Settings {
	return api.Settings{
		Listen:        settings.Web.Listen,
		AllowedHosts:  settings.Web.AllowedHosts,
		Version:       version,
		UPS:           settings.NUT.UPS,
		HealthyWithin: 3*max(settings.Poll.Interval, settings.Poll.OnBatteryInterval) + settings.NUT.ReadTimeout + settings.Probe.Timeout,
	}
}

func runAllUntilOneStops(ctx context.Context, services []func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, len(services))
	for _, service := range services {
		go func() {
			results <- service(ctx)
			cancel()
		}()
	}

	var errs []error
	for range services {
		errs = append(errs, <-results)
	}

	return errors.Join(errs...)
}
