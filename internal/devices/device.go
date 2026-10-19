package devices

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"time"

	"evigo/internal/config"
)

type Reachability int

const (
	Unknown Reachability = iota
	Offline
	Online
)

func (reachability Reachability) String() string {
	return [...]string{"unknown", "offline", "online"}[reachability]
}

type Device struct {
	ID          string
	Name        string
	Host        string
	MAC         net.HardwareAddr
	WakeAddress string
}

type Runner func(ctx context.Context, name string, arguments ...string) error

func FromConfig(settings config.Config) ([]*Device, error) {
	var devices []*Device
	for _, configured := range settings.Devices {
		mac, err := net.ParseMAC(configured.MAC)
		if err != nil || len(mac) != 6 || mac[0]&1 != 0 {
			return nil, fmt.Errorf("device %s: mac must be a unicast 6-byte address", configured.ID)
		}

		destination := settings.WOL
		if configured.WOL.Broadcast != "" {
			destination.Broadcast = configured.WOL.Broadcast
		}

		if configured.WOL.Port != 0 {
			destination.Port = configured.WOL.Port
		}

		devices = append(devices, &Device{
			ID:          configured.ID,
			Name:        configured.Name,
			Host:        configured.Host,
			MAC:         mac,
			WakeAddress: net.JoinHostPort(destination.Broadcast, strconv.Itoa(destination.Port)),
		})
	}

	return devices, nil
}

func (device *Device) Ping(ctx context.Context, run Runner, timeout time.Duration) Reachability {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	waitSeconds := strconv.FormatFloat(max(0.001, timeout.Seconds()*0.8), 'f', 3, 64)
	err := run(ctx, "ping", "-n", "-c", "1", "-W", waitSeconds, "--", device.Host)
	var exitError *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return Unknown
	case err == nil:
		return Online
	case errors.As(err, &exitError) && exitError.ExitCode() == 1:
		return Offline
	}

	return Unknown
}

func (device *Device) Wake(ctx context.Context) error {
	packet := append(bytes.Repeat([]byte{0xff}, 6), bytes.Repeat(device.MAC, 16)...)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(ctx, "udp4", device.WakeAddress)
	if err != nil {
		return err
	}

	defer connection.Close()
	deadline, _ := ctx.Deadline()
	connection.SetWriteDeadline(deadline)
	_, err = connection.Write(packet)

	return err
}

func RunCommand(ctx context.Context, name string, arguments ...string) error {
	return exec.CommandContext(ctx, name, arguments...).Run()
}
