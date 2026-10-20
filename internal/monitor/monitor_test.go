package monitor

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/devices"
	"github.com/Jack-Overflow/Evigo/internal/history"
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
	fail         bool
	saves        []map[string]recovery.Progress
	beforeSave   func(map[string]recovery.Progress)
	failHistory  bool
	historySaves int
	history      []history.Event
}

func (storage *fakeStorage) AppendHistory(_ context.Context, events []history.Event) error {
	if storage.failHistory {
		return errors.New("disk full")
	}

	storage.historySaves++
	storage.history = append(storage.history, events...)

	return nil
}

func (storage *fakeStorage) kinds(deviceID string) []history.Kind {
	var kinds []history.Kind
	for _, event := range storage.history {
		if event.DeviceID == deviceID {
			kinds = append(kinds, event.Kind)
		}
	}

	return kinds
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
		StablePowerDelay:  30 * time.Second,
		MinBatteryPercent: 25,
		RetryInterval:     30 * time.Second,
		DeviceTimeout:     10 * time.Minute,
		MaxReadingAge:     30 * time.Second,
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
	// Given a device due for waking, and storage that checks for a packet as it saves.
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

	// When the next pass runs.
	h.pass(10 * time.Second)

	// Then the attempt is saved before the packet arrives, and the packet is then sent.
	if !checkedBeforeSave || len(h.storage.saves) != savesBefore+1 || h.storage.saves[savesBefore]["server"].Attempts != 1 {
		t.Fatalf("saves %v", h.storage.saves)
	}
	if !h.packetReceived() {
		t.Fatal("no packet sent")
	}
}

func TestGivenDueDeviceWhenSavingFailsThenNoPacketIsSent(t *testing.T) {
	// Given a device due for waking and storage that cannot save.
	h := newHarness(t)
	h.outageThenStablePower()
	h.storage.fail = true

	// When the next pass runs.
	h.pass(10 * time.Second)

	// Then no packet is sent.
	if h.packetReceived() {
		t.Fatal("sent a packet without saving the attempt")
	}
}

func TestGivenDueDeviceWhenReadingIsTooOldAtSendTimeThenNoPacketIsSent(t *testing.T) {
	// Given a device due for waking, and a clock a minute ahead of the reading by the time it sends.
	h := newHarness(t)
	h.outageThenStablePower()

	h.clock = h.clock.Add(10 * time.Second)
	readingTime := h.clock
	h.ups.clock = &readingTime
	h.monitor.now = func() time.Time { return readingTime.Add(time.Minute) }

	// When the pass runs.
	h.monitor.Pass(context.Background())

	// Then no packet is sent from the expired reading.
	if h.packetReceived() {
		t.Fatal("sent a packet from a stale reading")
	}
}

func TestGivenUPSStateWhenPassFinishesThenPollIntervalMatchesPowerSource(t *testing.T) {
	// Given a monitor with 10s and 2s poll intervals.
	h := newHarness(t)

	// When passes run on mains, on battery and with the UPS unreachable.
	onMains := h.pass(0)
	h.ups.status = "OB DISCHRG"
	onBattery := h.pass(time.Second)
	h.ups.status = ""
	unavailable := h.pass(time.Second)

	// Then only the battery pass asks for the shorter interval.
	if onMains != 10*time.Second || onBattery != 2*time.Second || unavailable != 10*time.Second {
		t.Fatalf("intervals %s %s %s", onMains, onBattery, unavailable)
	}
}

func TestGivenTwoDueDevicesWhenReadingExpiresBetweenSendsThenOnlyFirstPacketIsSent(t *testing.T) {
	// Given two due devices and a clock that reaches the reading's expiry after the first send.
	h := newHarness(t)
	device := h.monitor.devices[0]
	reading := &nut.Reading{At: h.clock, Status: "OL", Battery: 100}
	h.monitor.now = func() time.Time {
		at := h.clock
		h.clock = h.clock.Add(h.monitor.settings.MaxReadingAge)

		return at
	}

	// When wakes are sent.
	h.monitor.sendWakes(context.Background(), []*devices.Device{device, device}, reading)

	// Then only the first packet goes out.
	if !h.packetReceived() || h.packetReceived() {
		t.Fatal("expected exactly one packet before the reading expired")
	}
}

func TestGivenSuccessfulWakeWhenDeviceAnswersThenMonitorFinishesRecovery(t *testing.T) {
	// Given a device that has been sent a wake packet.
	h := newHarness(t)
	h.outageThenStablePower()
	h.pass(10 * time.Second)
	if !h.packetReceived() {
		t.Fatal("no wake packet")
	}

	// When it answers ping on the next pass.
	h.online = true
	h.pass(10 * time.Second)

	// Then recovery finishes and the pending device is removed from storage.
	if len(h.monitor.recovery.Pending()) != 0 || len(h.storage.saves[len(h.storage.saves)-1]) != 0 {
		t.Fatal("confirmed wake and later reply did not finish recovery")
	}
}

func TestGivenUntrustedUPSStatusWhenReadThenLogsContainOnlyKnownFlags(t *testing.T) {
	// Given a UPS status containing text Evigo does not recognise.
	h := newHarness(t)
	var output bytes.Buffer
	h.monitor.log = slog.New(slog.NewJSONHandler(&output, nil))
	h.ups.status = "OL private-server-text CHRG private-server-text"

	// When a pass reads it.
	h.pass(0)

	// Then the log shows only known flags and UNKNOWN, and no recovery starts.
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
	if device.Reachability != devices.Offline || device.Pending == nil {
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

func TestGivenOutageAndRecoveryWhenPassesRunThenHistoryRecordsEachStep(t *testing.T) {
	// Given a device online on mains.
	h := newHarness(t)

	// When an outage happens, it is woken, and it answers.
	h.outageThenStablePower()
	h.pass(10 * time.Second)
	h.online = true
	h.pass(10 * time.Second)

	// Then history records each step for the device, and each UPS status change.
	want := []history.Kind{
		history.Reachability, history.DevicePending, history.Reachability,
		history.WakeSent, history.Reachability, history.DeviceRecovered,
	}
	if got := h.storage.kinds("server"); !slices.Equal(got, want) {
		t.Fatalf("history %v, want %v", got, want)
	}
	var statuses []string
	for _, event := range h.storage.history {
		if event.Kind == history.UPSStatus {
			statuses = append(statuses, event.Details.Status)
		}
	}
	if !slices.Equal(statuses, []string{"OL", "OB DISCHRG", "OL"}) {
		t.Fatalf("UPS statuses %v", statuses)
	}
}

func TestGivenNothingChangesWhenPassesRunThenPendingIsNeverSavedAndHistoryOnlyOnce(t *testing.T) {
	// Given a healthy UPS and a device that stays online.
	h := newHarness(t)
	h.online = true

	// When five passes run.
	for range 5 {
		h.pass(10 * time.Second)
	}

	// Then pending progress is never saved, and only the first pass, which saw everything for the first time, writes history.
	if len(h.storage.saves) != 0 || h.storage.historySaves != 1 {
		t.Fatalf("%d pending saves, history written %d times", len(h.storage.saves), h.storage.historySaves)
	}
}

func TestGivenHistoryUnavailableWhenPassesRunThenEventsAreKeptAndRecoveryContinues(t *testing.T) {
	// Given history that cannot be saved.
	h := newHarness(t)
	h.storage.failHistory = true

	// When an outage happens, power returns, and history recovers before the wake.
	h.outageThenStablePower()
	h.storage.failHistory = false
	h.pass(10 * time.Second)

	// Then the wake still happens, and the events saved late are not lost.
	if !h.packetReceived() {
		t.Fatal("history failure blocked an automatic wake")
	}
	if !slices.Contains(h.storage.kinds("server"), history.DevicePending) {
		t.Fatalf("unsaved events were lost: %v", h.storage.kinds("server"))
	}
}

func TestGivenWakeThatFailsToSendWhenPassRunsThenHistoryRecordsTheFailure(t *testing.T) {
	// Given a device due for waking with an address that cannot be sent to.
	h := newHarness(t)
	h.outageThenStablePower()
	h.monitor.devices[0].WakeAddress = "invalid address"

	// When the next pass runs.
	h.pass(10 * time.Second)

	// Then history records a failed automatic wake with its attempt number.
	last := h.storage.history[len(h.storage.history)-1]
	if last.Kind != history.WakeFailed || last.Details.Source != history.Automatic || last.Details.Attempt != 1 {
		t.Fatalf("last event %+v", last)
	}
}

type scriptedUPS struct {
	passes atomic.Int32
}

func (ups *scriptedUPS) Read(context.Context) (nut.Reading, error) {
	status := "OL"
	if ups.passes.Add(1) == 2 {
		status = "OB DISCHRG"
	}

	return nut.Reading{At: time.Now(), Status: status, Battery: 100}, nil
}

type slowFailingHistory struct {
	delay time.Duration
}

func (storage slowFailingHistory) SavePending(context.Context, map[string]recovery.Progress) error {
	return nil
}

func (storage slowFailingHistory) AppendHistory(context.Context, []history.Event) error {
	time.Sleep(storage.delay)

	return errors.New("database busy")
}

func TestGivenHistoryThatIsSlowAndFailsEveryPassWhenLoopRunsThenRecoveryStillWakesOnTime(t *testing.T) {
	// Given passes every 100ms, readings valid for 150ms, and history that takes 80ms to fail every pass.
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	mac, _ := net.ParseMAC("02:00:00:00:00:10")
	configured := []*devices.Device{{ID: "server", Host: "192.0.2.10", MAC: mac, WakeAddress: listener.LocalAddr().String()}}
	ups := &scriptedUPS{}
	ping := func(ctx context.Context, _ string, _ ...string) error {
		if ups.passes.Load() <= 1 {
			return nil
		}

		return exec.CommandContext(ctx, "sh", "-c", "exit 1").Run()
	}
	log := slog.New(slog.DiscardHandler)
	deviceRecovery := recovery.New(recovery.Settings{
		StablePowerDelay:  200 * time.Millisecond,
		MinBatteryPercent: 25,
		RetryInterval:     time.Minute,
		DeviceTimeout:     time.Minute,
		MaxReadingAge:     150 * time.Millisecond,
	}, configured, nil, log)
	settings := Settings{PollInterval: 100 * time.Millisecond, OnBatteryPollInterval: 100 * time.Millisecond, PingTimeout: time.Second, MaxReadingAge: 150 * time.Millisecond}
	loop := New(settings, ups, configured, deviceRecovery, slowFailingHistory{delay: 80 * time.Millisecond}, map[string]recovery.Progress{}, ping, log)

	// When the loop runs through an outage and power returns with the device off.
	ctx, stop := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(stopped)
	}()
	defer func() {
		stop()
		<-stopped
	}()

	// Then the device is still woken, because slow history no longer stretches the gap between passes.
	listener.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err = listener.ReadFrom(make([]byte, 200)); err != nil {
		t.Fatalf("no wake packet: %v", err)
	}
}
