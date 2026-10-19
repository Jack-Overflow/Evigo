package recovery

import (
	"log/slog"
	"net"
	"slices"
	"testing"
	"time"

	"evigo/internal/devices"
	"evigo/internal/nut"
)

var (
	mains     = &nut.Reading{Status: "OL", Battery: 100}
	lowMains  = &nut.Reading{Status: "OL CHRG", Battery: 10}
	onBattery = &nut.Reading{Status: "OB DISCHRG", Battery: 80}
	noReading *nut.Reading
	online    = devices.Online
	offline   = devices.Offline
	start     = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pollStep  = 10 * time.Second
)

type harness struct {
	t        *testing.T
	recovery *Recovery
	now      time.Time
}

func testSettings() Settings {
	return Settings{
		StablePowerDelay:    30 * time.Second,
		ShutdownGracePeriod: 2 * time.Minute,
		MinBatteryPercent:   25,
		RetryInterval:       30 * time.Second,
		DeviceTimeout:       10 * time.Minute,
		MaxReadingAge:       30 * time.Second,
	}
}

func newHarness(t *testing.T, saved map[string]Progress) *harness {
	configured := []*devices.Device{{ID: "server", MAC: net.HardwareAddr{0x02, 0, 0, 0, 0, 0x10}}}

	return &harness{t: t, recovery: New(testSettings(), configured, saved, slog.New(slog.DiscardHandler)), now: start}
}

func (h *harness) pass(after time.Duration, reading *nut.Reading, reachability devices.Reachability) []string {
	h.now = h.now.Add(after)
	if reading != nil {
		reading = reading.Copy()
		reading.At = h.now
	}
	due := h.recovery.Update(Pass{At: h.now, Reading: reading, Reachability: map[string]devices.Reachability{"server": reachability}})
	var dueIDs []string
	for _, device := range due {
		dueIDs = append(dueIDs, device.ID)
	}

	return dueIDs
}

func (h *harness) outage() {
	h.pass(0, mains, online)
	h.pass(pollStep, onBattery, online)
	h.pass(pollStep, onBattery, offline)
}

func (h *harness) passesUntilWake(reading *nut.Reading, limit int) int {
	for count := 1; count <= limit; count++ {
		if slices.Contains(h.pass(pollStep, reading, offline), "server") {
			h.recovery.WakeSent("server")

			return count
		}
	}

	return -1
}

func TestGivenHealthyStartupWithoutOutageWhenDeviceIsOfflineThenItIsNeverWoken(t *testing.T) {
	// Given a fresh start on mains with no saved state.
	h := newHarness(t, nil)

	// When the device stays offline for well past the stability delay.
	woken := h.passesUntilWake(mains, 20)

	// Then nothing is woken and nothing is pending.
	if woken != -1 || len(h.recovery.Pending()) != 0 {
		t.Fatal("woke a device without a recorded outage")
	}
}

func TestGivenDeviceOnlineBeforeOutageWhenPowerReturnsThenItIsWokenAfterTheStabilityDelay(t *testing.T) {
	// Given a device that was online on mains before the UPS went on battery.
	h := newHarness(t, nil)
	h.outage()

	// When mains returns and the device stays offline.
	woken := h.passesUntilWake(mains, 10)

	// Then the first wake comes once power has been safe for 30s (passes at 0, 10, 20, 30s).
	if woken != 4 {
		t.Fatalf("woken on pass %d", woken)
	}
}

func TestGivenDeviceOfflineBeforeOutageWhenPowerReturnsThenItStaysOff(t *testing.T) {
	// Given a device already off on mains before the outage.
	h := newHarness(t, nil)
	h.pass(0, mains, offline)
	h.pass(pollStep, onBattery, offline)

	// When power returns.
	woken := h.passesUntilWake(mains, 20)

	// Then it is never woken.
	if woken != -1 {
		t.Fatal("woke a device that was off before the outage")
	}
}

func TestGivenOutageWithStaleOnlineDevicesCheckWhenPowerReturnsThenNothingIsWoken(t *testing.T) {
	// Given the last mains observation is older than max_reading_age when the outage starts.
	h := newHarness(t, nil)
	h.pass(0, mains, online)
	h.pass(time.Minute, noReading, online)
	h.pass(pollStep, onBattery, offline)

	// When power returns.
	woken := h.passesUntilWake(mains, 20)

	// Then the stale baseline does not make the device eligible.
	if woken != -1 {
		t.Fatal("woke a device from a stale baseline")
	}
}

func TestGivenUnsafePowerWhenRecoveryIsPendingThenWakingWaitsForSafeStablePower(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		unsafe *nut.Reading
		passes int
	}{
		{"low battery", lowMains, 5},
		{"missing reading", noReading, 5},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Given a pending device after an outage.
			h := newHarness(t, nil)
			h.outage()

			// When power is unsafe for a while before becoming safe.
			for range scenario.passes {
				if due := h.pass(pollStep, scenario.unsafe, offline); len(due) != 0 {
					t.Fatal("woke while power was unsafe")
				}
			}
			woken := h.passesUntilWake(mains, 10)

			// Then the full stability delay still applies once power is safe.
			if woken != 4 {
				t.Fatalf("woken on safe pass %d", woken)
			}
		})
	}
}

func TestGivenStablePowerWhenAReadingIsMissingThenTheStabilityDelayRestarts(t *testing.T) {
	// Given a pending device and 20s of safe power.
	h := newHarness(t, nil)
	h.outage()
	h.pass(pollStep, mains, offline)
	h.pass(pollStep, mains, offline)

	// When one reading is missing.
	h.pass(pollStep, noReading, offline)

	// Then waking needs another full delay of safe readings.
	if woken := h.passesUntilWake(mains, 10); woken != 4 {
		t.Fatalf("woken on pass %d", woken)
	}
}

func TestGivenWakeSentWhenDeviceStaysOfflineThenRetriesFollowTheRetryInterval(t *testing.T) {
	// Given a device that has just been woken.
	h := newHarness(t, nil)
	h.outage()
	h.passesUntilWake(mains, 10)

	// When it stays offline for the next 30s.
	var retried []int
	for count := 1; count <= 6; count++ {
		if len(h.pass(pollStep, mains, offline)) != 0 {
			retried = append(retried, count)
		}
	}

	// Then it is retried every 30s.
	if !slices.Equal(retried, []int{3, 6}) || h.recovery.Pending()["server"].Attempts != 3 {
		t.Fatalf("retried on passes %v, progress %+v", retried, h.recovery.Pending()["server"])
	}
}

func TestGivenWokenDeviceWhenItAnswersPingThenRecoveryFinishes(t *testing.T) {
	// Given a device that has been woken once.
	h := newHarness(t, nil)
	h.outage()
	h.passesUntilWake(mains, 10)

	// When it answers ping.
	h.pass(pollStep, mains, online)

	// Then it is no longer pending.
	if len(h.recovery.Pending()) != 0 {
		t.Fatal("recovered device still pending")
	}
}

func TestGivenDeviceStillUpWhenPowerReturnsThenItIsOnlyRecoveredAfterShutdownGrace(t *testing.T) {
	// Given a device that kept answering ping through the outage.
	h := newHarness(t, nil)
	h.pass(0, mains, online)
	h.pass(pollStep, onBattery, online)

	// When power is stable and it is still online until the 2m shutdown grace is used up.
	for range 15 {
		h.pass(pollStep, mains, online)
	}
	stillPendingDuringGrace := len(h.recovery.Pending()) == 1
	h.pass(pollStep, mains, online)

	// Then it stays pending during grace and is only treated as recovered afterwards.
	if !stillPendingDuringGrace || len(h.recovery.Pending()) != 0 {
		t.Fatalf("pending during grace %t, after %v", stillPendingDuringGrace, h.recovery.Pending())
	}
}

func TestGivenDeviceShuttingDownLateWhenItGoesOfflineDuringGraceThenItIsWoken(t *testing.T) {
	// Given a device that still answers ping when power returns.
	h := newHarness(t, nil)
	h.pass(0, mains, online)
	h.pass(pollStep, onBattery, online)
	for range 4 {
		h.pass(pollStep, mains, online)
	}

	// When it finishes shutting down within grace.
	due := h.pass(pollStep, mains, offline)

	// Then it is woken.
	if !slices.Equal(due, []string{"server"}) {
		t.Fatalf("due = %v", due)
	}
}

func TestGivenDeviceNeverRespondsWhenTimeoutIsUsedThenRecoveryGivesUp(t *testing.T) {
	// Given a woken device that never comes back.
	h := newHarness(t, nil)
	h.outage()
	h.passesUntilWake(mains, 10)

	// When the 10m device timeout passes under stable power.
	for range 60 {
		h.pass(pollStep, mains, offline)
	}

	// Then it is no longer pending.
	if len(h.recovery.Pending()) != 0 {
		t.Fatalf("still pending: %+v", h.recovery.Pending())
	}
}

func TestGivenPendingDeviceWhenPowerIsUnsafeThenTimeoutIsNotSpent(t *testing.T) {
	// Given a device woken once.
	h := newHarness(t, nil)
	h.outage()
	h.passesUntilWake(mains, 10)
	before := h.recovery.Pending()["server"].TimeLeft

	// When the UPS cannot be read for an hour.
	for range 360 {
		h.pass(pollStep, noReading, offline)
	}

	// Then no device time is used.
	if after := h.recovery.Pending()["server"].TimeLeft; after != before {
		t.Fatalf("time left went from %s to %s", before, after)
	}
}

func TestGivenSavedProgressWhenRestartedThenRecoveryResumesAfterAFullStabilityDelay(t *testing.T) {
	// Given progress saved by a previous run.
	saved := map[string]Progress{"server": {Attempts: 1, LastAttempt: start.Add(-time.Hour), TimeLeft: 5 * time.Minute}}
	h := newHarness(t, saved)

	// When the new process sees safe power with the device offline.
	woken := h.passesUntilWake(mains, 10)

	// Then it continues waking after the full delay without resetting attempts.
	if woken != 4 || h.recovery.Pending()["server"].Attempts != 2 {
		t.Fatalf("woken on pass %d, progress %+v", woken, h.recovery.Pending()["server"])
	}
}

func TestGivenSavedProgressForRemovedDeviceWhenRestartedThenItIsDropped(t *testing.T) {
	// Given saved progress for a device no longer in the configuration.
	saved := map[string]Progress{"removed": {TimeLeft: time.Minute}}

	// When recovery is built.
	h := newHarness(t, saved)

	// Then only configured devices are pending.
	if len(h.recovery.Pending()) != 0 {
		t.Fatal("kept progress for an unconfigured device")
	}
}

func TestGivenStablePowerWhenStorageFailsThenTheStabilityDelayRestarts(t *testing.T) {
	// Given a pending device and 20s of safe power.
	h := newHarness(t, nil)
	h.outage()
	h.pass(pollStep, mains, offline)
	h.pass(pollStep, mains, offline)

	// When saving fails.
	h.recovery.StorageFailed()

	// Then waking needs another full delay.
	if woken := h.passesUntilWake(mains, 10); woken != 4 {
		t.Fatalf("woken on pass %d", woken)
	}
}

func TestGivenRepeatedOutageWhenDeviceIsPendingThenProgressIsKept(t *testing.T) {
	// Given a device woken once after an outage.
	h := newHarness(t, nil)
	h.outage()
	h.passesUntilWake(mains, 10)

	// When a second outage starts before it recovers.
	h.pass(pollStep, onBattery, offline)

	// Then its attempts are kept and grace is restored for the next return of power.
	progress := h.recovery.Pending()["server"]
	if progress.Attempts != 1 || progress.GraceLeft != 2*time.Minute {
		t.Fatalf("progress %+v", progress)
	}
}

func TestGivenStableRecoveryWhenPassesAreInterruptedThenTimersPauseAndStabilityRestarts(t *testing.T) {
	for _, gap := range []time.Duration{time.Hour, -time.Hour} {
		t.Run(gap.String(), func(t *testing.T) {
			// Given
			h := newHarness(t, nil)
			h.outage()
			h.passesUntilWake(mains, 10)
			before := h.recovery.Pending()["server"]

			// When
			due := h.pass(gap, mains, offline)
			after := h.recovery.Pending()["server"]

			// Then
			if len(due) != 0 || after.TimeLeft != before.TimeLeft || after.GraceLeft != before.GraceLeft {
				t.Fatalf("recovery advanced across a gap: due %v, before %+v, after %+v", due, before, after)
			}
			if woken := h.passesUntilWake(mains, 4); woken != 3 {
				t.Fatalf("woken on pass %d after the gap", woken)
			}
		})
	}
}

func TestGivenPendingRecoveryWhenReadingHasInvalidAgeThenTimersPauseAndStabilityRestarts(t *testing.T) {
	for _, age := range []time.Duration{time.Minute, -time.Minute} {
		t.Run(age.String(), func(t *testing.T) {
			// Given
			h := newHarness(t, nil)
			h.outage()
			h.passesUntilWake(mains, 10)
			before := h.recovery.Pending()["server"]
			h.now = h.now.Add(pollStep)
			reading := mains.Copy()
			reading.At = h.now.Add(-age)

			// When
			due := h.recovery.Update(Pass{At: h.now, Reading: reading, Reachability: map[string]devices.Reachability{"server": offline}})

			// Then
			if len(due) != 0 || h.recovery.Pending()["server"] != before {
				t.Fatal("invalid-age reading advanced recovery")
			}
			if woken := h.passesUntilWake(mains, 5); woken != 4 {
				t.Fatalf("woken on pass %d", woken)
			}
		})
	}
}

func TestGivenUnsentAttemptWhenDeviceAnswersDuringGraceThenItRemainsPending(t *testing.T) {
	// Given
	h := newHarness(t, nil)
	h.outage()
	for range 4 {
		h.pass(pollStep, mains, offline)
	}

	// When
	h.pass(pollStep, mains, online)

	// Then
	if len(h.recovery.Pending()) != 1 {
		t.Fatal("unsent attempt bypassed shutdown grace")
	}
}

func TestGivenPreviouslyWokenDeviceWhenAnotherOutageOccursThenNewGraceApplies(t *testing.T) {
	// Given
	h := newHarness(t, nil)
	h.outage()
	h.passesUntilWake(mains, 10)

	// When
	h.pass(pollStep, onBattery, online)
	for range 4 {
		h.pass(pollStep, mains, online)
	}

	// Then
	if len(h.recovery.Pending()) != 1 {
		t.Fatal("previous outage's wake bypassed new shutdown grace")
	}
	if due := h.pass(pollStep, mains, offline); !slices.Equal(due, []string{"server"}) {
		t.Fatalf("late shutdown was not woken: %v", due)
	}
}

func TestGivenFutureSavedAttemptWhenRestartedThenRetryFollowsFullStabilityDelay(t *testing.T) {
	// Given
	h := newHarness(t, map[string]Progress{"server": {Attempts: 1, LastAttempt: start.Add(time.Hour), TimeLeft: time.Minute}})

	// When
	woken := h.passesUntilWake(mains, 5)

	// Then
	if woken != 4 || h.recovery.Pending()["server"].Attempts != 2 {
		t.Fatalf("future timestamp suppressed retry: pass %d, pending %v", woken, h.recovery.Pending())
	}
}

func TestGivenSavedAttemptWhenRestartedThenAnOnlineReplyStillHonoursGrace(t *testing.T) {
	// Given
	h := newHarness(t, map[string]Progress{"server": {Attempts: 1, LastAttempt: start.Add(-time.Minute), TimeLeft: time.Minute, GraceLeft: time.Minute}})

	// When
	for range 4 {
		h.pass(pollStep, mains, online)
	}

	// Then
	if len(h.recovery.Pending()) != 1 {
		t.Fatal("restored attempt was treated as a confirmed send")
	}
}

func TestGivenPendingDeviceWhenPingIsUnknownThenNoWakeIsScheduled(t *testing.T) {
	// Given
	h := newHarness(t, nil)
	h.outage()

	// When
	for range 10 {
		due := h.pass(pollStep, mains, devices.Unknown)

		// Then
		if len(due) != 0 {
			t.Fatal("unknown ping scheduled a wake")
		}
	}
}
