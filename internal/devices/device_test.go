package devices

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os/exec"
	"testing"
	"time"

	"evigo/internal/config"
)

func exitingWith(code string) Runner {
	return func(ctx context.Context, _ string, _ ...string) error {
		return exec.CommandContext(ctx, "sh", "-c", "exit "+code).Run()
	}
}

func TestGivenConfigWhenDevicesBuiltThenMACAndWakeAddressAreResolvedOnce(t *testing.T) {
	// Given one device using the global WOL destination and one overriding it.
	settings := config.Defaults()
	settings.Devices = []config.Device{
		{ID: "server", Name: "Server", Host: "192.0.2.10", MAC: "02:00:00:00:00:10"},
		{ID: "storage", Name: "Storage", Host: "192.0.2.11", MAC: "02:00:00:00:00:11", WOL: config.WOL{Broadcast: "192.0.2.255"}},
	}

	// When devices are built from the configuration.
	built, err := FromConfig(settings)
	// Then each device carries its parsed MAC and final wake address.
	if err != nil {
		t.Fatal(err)
	}
	if built[0].MAC.String() != "02:00:00:00:00:10" || built[0].WakeAddress != "255.255.255.255:9" {
		t.Fatalf("default device = %+v", built[0])
	}
	if built[1].WakeAddress != "192.0.2.255:9" {
		t.Fatalf("override device = %+v", built[1])
	}
}

func TestGivenPingOutcomeWhenDevicePingedThenOnlyExitOneMeansOffline(t *testing.T) {
	for _, scenario := range []struct {
		name string
		run  Runner
		want Reachability
	}{
		{"reply", exitingWith("0"), Online},
		{"no reply", exitingWith("1"), Offline},
		{"ping error", exitingWith("2"), Unknown},
		{"missing ping", func(context.Context, string, ...string) error { return errors.New("not found") }, Unknown},
		{"timeout", func(ctx context.Context, _ string, _ ...string) error { <-ctx.Done(); return ctx.Err() }, Unknown},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Given a device and a simulated ping result.
			device := &Device{ID: "server", Host: "192.0.2.10"}

			// When the device is pinged.
			got := device.Ping(context.Background(), scenario.run, 50*time.Millisecond)

			// Then the result maps to the expected reachability.
			if got != scenario.want {
				t.Fatalf("got %s, want %s", got, scenario.want)
			}
		})
	}
}

func TestGivenLoopbackListenerWhenDeviceWokenThenMagicPacketIsSent(t *testing.T) {
	// Given a UDP listener standing in for the broadcast address.
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	mac, _ := net.ParseMAC("02:00:00:00:00:10")
	device := &Device{ID: "server", MAC: mac, WakeAddress: listener.LocalAddr().String()}

	// When the device is woken.
	if err = device.Wake(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Then the listener receives six 0xff bytes followed by the MAC sixteen times.
	packet := make([]byte, 200)
	listener.SetReadDeadline(time.Now().Add(time.Second))
	size, _, err := listener.ReadFrom(packet)
	if err != nil {
		t.Fatal(err)
	}
	want := append(bytes.Repeat([]byte{0xff}, 6), bytes.Repeat(mac, 16)...)
	if !bytes.Equal(packet[:size], want) {
		t.Fatalf("packet = %x", packet[:size])
	}
}

func TestGivenInvalidMACWhenDevicesBuiltThenConstructionFails(t *testing.T) {
	for _, mac := range []string{"not-a-mac", "ff:ff:ff:ff:ff:ff", "01:00:5e:00:00:01", "02:00:00:00:00:00:00:10"} {
		t.Run(mac, func(t *testing.T) {
			// Given a device with a MAC that cannot be woken.
			settings := config.Defaults()
			settings.Devices = []config.Device{{ID: "server", Name: "Server", Host: "192.0.2.10", MAC: mac}}

			// When devices are built.
			_, err := FromConfig(settings)

			// Then construction fails.
			if err == nil {
				t.Fatal("accepted an unusable MAC")
			}
		})
	}
}
