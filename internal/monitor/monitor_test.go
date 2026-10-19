package monitor

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/devices"
	"github.com/Jack-Overflow/Evigo/internal/nut"
	"github.com/Jack-Overflow/Evigo/internal/recovery"
)

type fakeUPS struct {
	status string
	clock  *time.Time
}

func (ups *fakeUPS) Read(context.Context) (nut.Reading, error) {
	if ups.status == "" {
		return nut.Reading{}, errors.New("connection refused")
	}

	return nut.Reading{At: *ups.clock, Status: ups.status, Battery: 100}, nil
}

type fakeStorage struct {
	fail       bool
	saves      []map[string]recovery.Progress
	beforeSave func(map[string]recovery.Progress)
}

func (storage *fakeStorage) SavePending(_ context.Context, pending map[string]recovery.Progress) error {
	if storage.beforeSave != nil {
		storage.beforeSave(pending)
	}
	if storage.fail {
		return errors.New("disk full")
	}

	storage.saves = append(storage.saves, pending)

	return nil
}

type harness struct {
	t        *testing.T
	monitor  *Monitor
	ups      *fakeUPS
	storage  *fakeStorage
	listener net.PacketConn
	online   bool
	clock    time.Time
}

func newHarness(t *testing.T) *harness {
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	mac, _ := net.ParseMAC("02:00:00:00:00:10")
	configured := []*devices.Device{{ID: "server", Host: "192.0.2.10", MAC: mac, WakeAddress: listener.LocalAddr().String()}}
	h := &harness{t: t, storage: &fakeStorage{}, listener: listener, clock: time.Now()}
	h.ups = &fakeUPS{status: "OL", clock: &h.clock}
	ping := func(ctx context.Context, _ string, _ ...string) error {
		if h.online {
			return nil
		}

		return exec.CommandContext(ctx, "sh", "-c", "exit 1").Run()
	}
	log := slog.New(slog.DiscardHandler)
	deviceRecovery := recovery.New(recovery.Settings{
		StablePowerDelay:    30 * time.Second,
		ShutdownGracePeriod: 2 * time.Minute,
		MinBatteryPercent:   25,
		RetryInterval:       30 * time.Second,
		DeviceTimeout:       10 * time.Minute,
		MaxReadingAge:       30 * time.Second,
	}, configured, nil, log)
	settings := Settings{PollInterval: 10 * time.Second, OnBatteryPollInterval: 2 * time.Second, PingTimeout: 2 * time.Second, MaxReadingAge: 30 * time.Second}
	h.monitor = New(settings, h.ups, configured, deviceRecovery, h.storage, map[string]recovery.Progress{}, ping, log)
	h.monitor.now = func() time.Time { return h.clock }

	return h
}

func (h *harness) pass(after time.Duration) time.Duration {
	h.clock = h.clock.Add(after)

	return h.monitor.Pass(context.Background())
}

func (h *harness) outageThenStablePower() {
	h.online = true
	h.pass(0)
	h.ups.status = "OB DISCHRG"
	h.pass(2 * time.Second)
	h.online = false
	h.ups.status = "OL"
	for range 3 {
		h.pass(10 * time.Second)
	}
}

func (h *harness) packetReceived() bool {
	h.listener.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, err := h.listener.ReadFrom(make([]byte, 200))

	return err == nil
}

func TestGivenDueDeviceWhenPassRunsThenAttemptIsSavedBeforeThePacketIsSent(t *testing.T) {
	// Given
	h := newHarness(t)
	h.outageThenStablePower()
	savesBefore := len(h.storage.saves)
	checkedBeforeSave := false
	h.storage.beforeSave = func(pending map[string]recovery.Progress) {
		if pending["server"].Attempts != 1 || h.packetReceived() {
			t.Fatal("attempt was not recorded before sending")
		}
		checkedBeforeSave = true
	}

	// When
	h.pass(10 * time.Second)

	// Then
	if !checkedBeforeSave || len(h.storage.saves) != savesBefore+1 || h.storage.saves[savesBefore]["server"].Attempts != 1 {
		t.Fatalf("saves %v", h.storage.saves)
	}
	if !h.packetReceived() {
		t.Fatal("no packet sent")
	}
}

func TestGivenDueDeviceWhenSavingFailsThenNoPacketIsSent(t *testing.T) {
	// Given
	h := newHarness(t)
	h.outageThenStablePower()
	h.storage.fail = true

	// When
	h.pass(10 * time.Second)

	// Then
	if h.packetReceived() {
		t.Fatal("sent a packet without saving the attempt")
	}
}

func TestGivenDueDeviceWhenReadingIsTooOldAtSendTimeThenNoPacketIsSent(t *testing.T) {
	// Given
	h := newHarness(t)
	h.outageThenStablePower()

	h.clock = h.clock.Add(10 * time.Second)
	readingTime := h.clock
	h.ups.clock = &readingTime
	h.monitor.now = func() time.Time { return readingTime.Add(time.Minute) }

	// When
	h.monitor.Pass(context.Background())

	// Then
	if h.packetReceived() {
		t.Fatal("sent a packet from a stale reading")
	}
}

func TestGivenUPSStateWhenPassFinishesThenPollIntervalMatchesPowerSource(t *testing.T) {
	// Given
	h := newHarness(t)

	// When
	onMains := h.pass(0)
	h.ups.status = "OB DISCHRG"
	onBattery := h.pass(time.Second)
	h.ups.status = ""
	unavailable := h.pass(time.Second)

	// Then
	if onMains != 10*time.Second || onBattery != 2*time.Second || unavailable != 10*time.Second {
		t.Fatalf("intervals %s %s %s", onMains, onBattery, unavailable)
	}
}

func TestGivenNoChangesWhenPassesRunThenNothingIsSaved(t *testing.T) {
	// Given
	h := newHarness(t)

	// When
	for range 5 {
		h.pass(10 * time.Second)
	}

	// Then
	if len(h.storage.saves) != 0 {
		t.Fatalf("%d unnecessary saves", len(h.storage.saves))
	}
}

func TestGivenTwoDueDevicesWhenReadingExpiresBetweenSendsThenOnlyFirstPacketIsSent(t *testing.T) {
	// Given
	h := newHarness(t)
	device := h.monitor.devices[0]
	reading := &nut.Reading{At: h.clock, Status: "OL", Battery: 100}
	h.monitor.now = func() time.Time {
		at := h.clock
		h.clock = h.clock.Add(h.monitor.settings.MaxReadingAge)

		return at
	}

	// When
	h.monitor.sendWakes(context.Background(), []*devices.Device{device, device}, reading)

	// Then
	if !h.packetReceived() || h.packetReceived() {
		t.Fatal("expected exactly one packet before the reading expired")
	}
}

func TestGivenSkippedFirstWakeWhenDeviceAnswersDuringGraceThenItRemainsPending(t *testing.T) {
	for _, reason := range []string{"storage failure", "expired reading", "send failure"} {
		t.Run(reason, func(t *testing.T) {
			// Given
			h := newHarness(t)
			h.outageThenStablePower()
			switch reason {
			case "storage failure":
				h.storage.fail = true
			case "expired reading":
				h.storage.beforeSave = func(map[string]recovery.Progress) { h.clock = h.clock.Add(time.Minute) }
			case "send failure":
				h.monitor.devices[0].WakeAddress = "invalid address"
			}

			// When
			h.pass(10 * time.Second)
			h.storage.fail = false
			h.storage.beforeSave = nil
			h.online = true
			for range 4 {
				h.pass(10 * time.Second)
			}

			// Then
			if h.packetReceived() || len(h.monitor.recovery.Pending()) != 1 {
				t.Fatal("skipped send bypassed grace or emitted a packet")
			}
		})
	}
}

func TestGivenSuccessfulWakeWhenDeviceAnswersThenMonitorFinishesRecovery(t *testing.T) {
	// Given
	h := newHarness(t)
	h.outageThenStablePower()
	h.pass(10 * time.Second)
	if !h.packetReceived() {
		t.Fatal("no wake packet")
	}

	// When
	h.online = true
	h.pass(10 * time.Second)

	// Then
	if len(h.monitor.recovery.Pending()) != 0 || len(h.storage.saves[len(h.storage.saves)-1]) != 0 {
		t.Fatal("confirmed wake and later reply did not finish recovery")
	}
}

func TestGivenUntrustedUPSStatusWhenReadThenLogsContainOnlyKnownFlags(t *testing.T) {
	// Given
	h := newHarness(t)
	var output bytes.Buffer
	h.monitor.log = slog.New(slog.NewJSONHandler(&output, nil))
	h.ups.status = "OL private-server-text CHRG private-server-text"

	// When
	h.pass(0)

	// Then
	if strings.Contains(output.String(), "private-server-text") || !strings.Contains(output.String(), "OL UNKNOWN CHRG") {
		t.Fatalf("unexpected log: %s", output.String())
	}
	if len(h.monitor.recovery.Pending()) != 0 {
		t.Fatal("unknown status established recovery")
	}
}

func TestGivenPendingDeviceWhenSnapshotReadThenItReflectsTheLatestPass(t *testing.T) {
	// Given a device pending after an outage, with power not yet stable.
	h := newHarness(t)
	h.outageThenStablePower()

	// When the status is read.
	latest := h.monitor.Snapshot()

	// Then it shows the latest reading, reachability and recovery progress.
	device := latest.Devices[0]
	if device.Reachability != devices.Offline || device.Pending == nil || device.WakeSent {
		t.Fatalf("device status %+v", device)
	}
	if latest.Reading == nil || !latest.CheckedAt.Equal(h.clock) || !latest.StorageOK {
		t.Fatalf("snapshot %+v", latest)
	}
	if latest.PowerBlock(h.clock) != recovery.Stabilising || latest.PowerBlock(h.clock.Add(10*time.Second)) != recovery.NoBlock {
		t.Fatal("power block does not follow the stability delay")
	}
}

func TestGivenSavingFailsWhenSnapshotReadThenStorageIsReportedUnavailable(t *testing.T) {
	// Given a device due for waking and storage that cannot save.
	h := newHarness(t)
	h.outageThenStablePower()
	h.storage.fail = true

	// When the next pass runs.
	h.pass(10 * time.Second)

	// Then the status shows storage as unavailable.
	if h.monitor.Snapshot().StorageOK {
		t.Fatal("storage failure not reported")
	}
}

func TestGivenNoPassYetWhenSnapshotReadThenItHasNoReadingAndUnknownDevices(t *testing.T) {
	// Given a monitor that has not run a pass.
	h := newHarness(t)

	// When the status is read.
	latest := h.monitor.Snapshot()

	// Then nothing has been checked yet.
	if !latest.CheckedAt.IsZero() || latest.Reading != nil || latest.Devices[0].Reachability != devices.Unknown {
		t.Fatalf("snapshot %+v", latest)
	}
}

func TestGivenSnapshotReadWhenCallerChangesItThenTheMonitorsDevicesAreUnaffected(t *testing.T) {
	// Given a monitor after a pass.
	h := newHarness(t)
	h.pass(0)
	wakeAddress := h.monitor.devices[0].WakeAddress

	// When a caller changes the device in its snapshot.
	latest := h.monitor.Snapshot()
	latest.Devices[0].Device.WakeAddress = "192.0.2.99:9"
	latest.Devices[0].Device.MAC[0] = 0xff

	// Then the device the monitor pings and wakes is unchanged.
	if h.monitor.devices[0].WakeAddress != wakeAddress || h.monitor.devices[0].MAC[0] == 0xff {
		t.Fatal("caller changed the device the monitor wakes")
	}
}
