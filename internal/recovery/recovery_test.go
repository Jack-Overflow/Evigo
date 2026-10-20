package recovery

import (
	"log/slog"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/devices"
	"github.com/Jack-Overflow/Evigo/internal/history"
	"github.com/Jack-Overflow/Evigo/internal/nut"
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
	events   []history.Event
}

func testSettings() Settings {
	return Settings{
		StablePowerDelay:  30 * time.Second,
		MinBatteryPercent: 25,
		RetryInterval:     30 * time.Second,
		DeviceTimeout:     10 * time.Minute,
		MaxReadingAge:     30 * time.Second,
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
	outcome := h.recovery.Update(Pass{At: h.now, Reading: reading, Reachability: map[string]devices.Reachability{"server": reachability}})
	h.events = append(h.events, outcome.Events...)
	var dueIDs []string
	for _, device := range outcome.Wake {
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

func TestGivenLowBatteryWhenRecoveryIsPendingThenWakingWaitsForSafeStablePower(t *testing.T) {
	// Given a pending device after an outage.
	h := newHarness(t, nil)
	h.outage()

	// When the battery is below the minimum for a while before power becomes safe.
	for range 5 {
		if due := h.pass(pollStep, lowMains, offline); len(due) != 0 {
			t.Fatal("woke while the battery was low")
		}
	}
	woken := h.passesUntilWake(mains, 10)

	// Then the full stability delay still applies once power is safe.
	if woken != 4 {
		t.Fatalf("woken on safe pass %d", woken)
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

func TestGivenDeviceThatStayedOnWhenWakingIsAllowedThenItIsDoneWithoutAWake(t *testing.T) {
	// Given a device that kept answering ping through the outage.
	h := newHarness(t, nil)
	h.pass(0, mains, online)
	h.pass(pollStep, onBattery, online)

	// When power returns and stays safe for the stability delay with the device still on.
	var woken []string
	for range 4 {
		woken = append(woken, h.pass(pollStep, mains, online)...)
	}

	// Then it is no longer pending, was never woken, and is reported as already on.
	if len(woken) != 0 || len(h.recovery.Pending()) != 0 {
		t.Fatalf("woken %v, pending %v", woken, h.recovery.Pending())
	}
	if last := h.events[len(h.events)-1]; last.Kind != history.DeviceAlreadyOn || last.DeviceID != "server" {
		t.Fatalf("events %+v", h.events)
	}
}

func TestGivenDeviceStillShuttingDownWhenMainsReturnsThenItIsWokenIfOffWhenWakingIsAllowed(t *testing.T) {
	// Given a device still answering ping when mains returns.
	h := newHarness(t, nil)
	h.pass(0, mains, online)
	h.pass(pollStep, onBattery, online)
	h.pass(pollStep, mains, online)

	// When it finishes shutting down before the stability delay ends.
	woken := h.passesUntilWake(mains, 5)

	// Then it is woken as soon as waking is allowed.
	if woken != 3 {
		t.Fatalf("woken on pass %d", woken)
	}
}

func TestGivenDeviceStillOnWhenWakingIsAllowedThenALaterShutdownIsNotWoken(t *testing.T) {
	// Given a device still on when waking becomes allowed after an outage.
	h := newHarness(t, nil)
	h.pass(0, mains, online)
	h.pass(pollStep, onBattery, online)
	for range 4 {
		h.pass(pollStep, mains, online)
	}

	// When it goes off afterwards.
	woken := h.passesUntilWake(mains, 10)

	// Then it is left off, as a device turned off on purpose would be.
	if woken != -1 {
		t.Fatalf("woken on pass %d", woken)
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

	// Then its attempts are kept.
	progress := h.recovery.Pending()["server"]
	if progress.Attempts != 1 {
		t.Fatalf("progress %+v", progress)
	}
}

func TestGivenStableRecoveryWhenPassesAreInterruptedThenTimersPauseAndStabilityRestarts(t *testing.T) {
	for _, gap := range []time.Duration{time.Hour, -time.Hour} {
		t.Run(gap.String(), func(t *testing.T) {
			// Given a woken device during stable power.
			h := newHarness(t, nil)
			h.outage()
			h.passesUntilWake(mains, 10)
			before := h.recovery.Pending()["server"]

			// When the next pass comes after a long gap or with time going backwards.
			due := h.pass(gap, mains, offline)
			after := h.recovery.Pending()["server"]

			// Then nothing is woken or spent across the gap, and the full stability delay applies again.
			if len(due) != 0 || after.TimeLeft != before.TimeLeft {
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
			// Given a woken device during stable power.
			h := newHarness(t, nil)
			h.outage()
			h.passesUntilWake(mains, 10)
			before := h.recovery.Pending()["server"]
			h.now = h.now.Add(pollStep)
			reading := mains.Copy()
			reading.At = h.now.Add(-age)

			// When a pass arrives with a reading that is too old or from the future.
			due := h.recovery.Update(Pass{At: h.now, Reading: reading, Reachability: map[string]devices.Reachability{"server": offline}}).Wake

			// Then nothing is woken or spent, and the full stability delay applies again.
			if len(due) != 0 || h.recovery.Pending()["server"] != before {
				t.Fatal("invalid-age reading advanced recovery")
			}
			if woken := h.passesUntilWake(mains, 5); woken != 4 {
				t.Fatalf("woken on pass %d", woken)
			}
		})
	}
}

func TestGivenFutureSavedAttemptWhenRestartedThenRetryFollowsFullStabilityDelay(t *testing.T) {
	// Given a saved attempt timestamped an hour in the future.
	h := newHarness(t, map[string]Progress{"server": {Attempts: 1, LastAttempt: start.Add(time.Hour), TimeLeft: time.Minute}})

	// When Evigo restarts and power is stable.
	woken := h.passesUntilWake(mains, 5)

	// Then the retry happens after the normal stability delay.
	if woken != 4 || h.recovery.Pending()["server"].Attempts != 2 {
		t.Fatalf("future timestamp suppressed retry: pass %d, pending %v", woken, h.recovery.Pending())
	}
}

func TestGivenSavedProgressWhenRestartedAndTheDeviceIsOnThenItIsDoneOnceWakingIsAllowed(t *testing.T) {
	// Given progress saved by a previous run.
	h := newHarness(t, map[string]Progress{"server": {Attempts: 1, LastAttempt: start.Add(-time.Minute), TimeLeft: time.Minute}})

	// When Evigo restarts and the device answers ping through the stability delay.
	pendingBeforeWakingAllowed := false
	for count := 1; count <= 4; count++ {
		h.pass(pollStep, mains, online)
		if count == 3 {
			pendingBeforeWakingAllowed = len(h.recovery.Pending()) == 1
		}
	}

	// Then it stays pending until waking is allowed, and is then done.
	if !pendingBeforeWakingAllowed || len(h.recovery.Pending()) != 0 {
		t.Fatalf("pending before waking allowed %t, after %v", pendingBeforeWakingAllowed, h.recovery.Pending())
	}
}

func TestGivenPendingDeviceWhenPingIsUnknownThenNoWakeIsScheduled(t *testing.T) {
	// Given a device pending after an outage.
	h := newHarness(t, nil)
	h.outage()

	// When power is stable but its ping result is unknown.
	for range 10 {
		due := h.pass(pollStep, mains, devices.Unknown)

		// Then no wake is scheduled.
		if len(due) != 0 {
			t.Fatal("unknown ping scheduled a wake")
		}
	}
}

func TestGivenPowerStateWhenBlockCheckedThenTheReasonMatchesWhyWakingIsNotAllowed(t *testing.T) {
	stableSince := start.Add(-time.Minute)
	for _, scenario := range []struct {
		name      string
		reading   *nut.Reading
		readAge   time.Duration
		safeSince time.Time
		want      Block
	}{
		{"stable mains", mains, 0, stableSince, NoBlock},
		{"just restored", mains, 0, start.Add(-10 * time.Second), Stabilising},
		{"never safe", mains, 0, time.Time{}, Stabilising},
		{"on battery", onBattery, 0, stableSince, OnBattery},
		{"low battery", lowMains, 0, stableSince, BatteryLow},
		{"alarm flag", &nut.Reading{Status: "OL ALARM", Battery: 100}, 0, stableSince, UnsafeStatus},
		{"settling onto mains", &nut.Reading{Status: "OL CHRG DISCHRG", Battery: 60}, 0, stableSince, Settling},
		{"no reading", nil, 0, stableSince, NoReading},
		{"expired reading", mains, time.Minute, stableSince, NoReading},
		{"reading exactly at max age", mains, 30 * time.Second, stableSince, NoReading},
		{"reading just inside max age", mains, 30*time.Second - time.Nanosecond, stableSince, NoBlock},
		{"reading from the future", mains, -time.Minute, stableSince, NoReading},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Given a reading of this age and a time power became safe.
			var reading *nut.Reading
			if scenario.reading != nil {
				reading = scenario.reading.Copy()
				reading.At = start.Add(-scenario.readAge)
			}

			// When the block is worked out.
			got := testSettings().PowerBlock(start, reading, scenario.safeSince)

			// Then it names why waking is not allowed, or nothing when it is.
			if got != scenario.want {
				t.Fatalf("got %q, want %q", got, scenario.want)
			}
		})
	}
}

func TestGivenOutageAndRecoveryWhenUpdatesRunThenEachTransitionIsReportedOnce(t *testing.T) {
	// Given a device online on mains.
	h := newHarness(t, nil)

	// When an outage happens, the device is woken, it answers, and power stays stable.
	h.outage()
	h.passesUntilWake(mains, 10)
	h.pass(pollStep, mains, online)
	h.pass(pollStep, mains, online)

	// Then it is no longer pending, and pending and recovered were each reported once, with the attempt count.
	var kinds []history.Kind
	for _, event := range h.events {
		kinds = append(kinds, event.Kind)
	}
	if len(h.recovery.Pending()) != 0 {
		t.Fatal("recovered device still pending")
	}
	if !slices.Equal(kinds, []history.Kind{history.DeviceWatched, history.DeviceRecovered}) || h.events[1].Details.Attempt != 1 {
		t.Fatalf("events %+v", h.events)
	}
}

func TestGivenDeviceThatNeverAnswersWhenTimeoutRunsOutThenRecoveryGivesUpAndReportsIt(t *testing.T) {
	// Given a woken device that never comes back.
	h := newHarness(t, nil)
	h.outage()
	h.passesUntilWake(mains, 10)

	// When its timeout runs out.
	for range 70 {
		h.pass(pollStep, mains, offline)
	}

	// Then it is no longer pending and giving up is reported.
	if len(h.recovery.Pending()) != 0 {
		t.Fatalf("still pending: %+v", h.recovery.Pending())
	}
	if last := h.events[len(h.events)-1]; last.Kind != history.DeviceGaveUp || last.DeviceID != "server" {
		t.Fatalf("events %+v", h.events)
	}
}

func TestGivenPowerReturningWhenTheStabilityDelayPassesThenRecoveryStartsSpendingTimeLeft(t *testing.T) {
	// Given a pending device after an outage.
	h := newHarness(t, nil)
	h.outage()

	// When power is safe for 20s, then for the full 30s stability delay.
	h.pass(pollStep, mains, offline)
	h.pass(pollStep, mains, offline)
	h.pass(pollStep, mains, offline)
	spendingBeforeStable := h.recovery.SpendingTimeLeft()
	h.pass(pollStep, mains, offline)

	// Then time left is only being spent once the delay has passed.
	if spendingBeforeStable || !h.recovery.SpendingTimeLeft() {
		t.Fatalf("spending before stable %t, after %t", spendingBeforeStable, h.recovery.SpendingTimeLeft())
	}
}

func TestGivenPendingDeviceWhenPingBecomesUnknownAfterAWakeThenItIsStillBeingWoken(t *testing.T) {
	// Given a device woken once after an outage.
	h := newHarness(t, nil)
	h.outage()
	h.passesUntilWake(mains, 10)

	// When its next ping result is unknown.
	due := h.pass(pollStep, mains, devices.Unknown)

	// Then no packet is due, but it is still being woken rather than watched.
	progress, pending := h.recovery.Pending()["server"]
	if len(due) != 0 || !pending || progress.State(devices.Unknown) != Waking {
		t.Fatalf("due %v, pending %t, progress %+v", due, pending, progress)
	}
}
