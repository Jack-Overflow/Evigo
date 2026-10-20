package snapshot

import (
	"net"
	"testing"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/devices"
	"github.com/Jack-Overflow/Evigo/internal/nut"
	"github.com/Jack-Overflow/Evigo/internal/recovery"
)

func sampleSnapshot() Snapshot {
	mac, _ := net.ParseMAC("02:00:00:00:00:10")

	return Snapshot{
		CheckedAt: time.Unix(1_700_000_000, 0),
		Reading:   &nut.Reading{Status: "OL", Battery: 100, Values: map[string]string{"ups.status": "OL"}},
		StorageOK: true,
		Devices: []Device{{
			Device:       devices.Device{ID: "server", MAC: mac, WakeAddress: "192.0.2.255:9"},
			Reachability: devices.Offline,
			Pending:      &recovery.Progress{Attempts: 1},
		}},
	}
}

func TestGivenSnapshotWhenCopyIsChangedThenTheOriginalIsUnaffected(t *testing.T) {
	// Given a snapshot and a copy of it.
	original := sampleSnapshot()
	copied := original.Copy()

	// When everything in the copy is changed.
	copied.Reading.Status = "OB"
	copied.Reading.Values["ups.status"] = "OB"
	copied.Devices[0].Device.WakeAddress = "192.0.2.99:9"
	copied.Devices[0].Device.MAC[0] = 0xff
	copied.Devices[0].Pending.Attempts = 99

	// Then the original still holds what it had.
	device := original.Devices[0]
	if original.Reading.Status != "OL" || original.Reading.Values["ups.status"] != "OL" {
		t.Fatalf("reading changed: %+v", original.Reading)
	}
	if device.Device.WakeAddress != "192.0.2.255:9" || device.Device.MAC[0] == 0xff || device.Pending.Attempts != 1 {
		t.Fatalf("device changed: %+v", device)
	}
}

func TestGivenEmptySnapshotWhenCopiedThenTheCopyIsEmptyToo(t *testing.T) {
	// Given a snapshot from before any pass.
	var empty Snapshot

	// When it is copied.
	copied := empty.Copy()

	// Then nothing is invented.
	if copied.Reading != nil || len(copied.Devices) != 0 || !copied.CheckedAt.IsZero() {
		t.Fatalf("copy %+v", copied)
	}
}

func TestGivenSnapshotWhenBlockCheckedLaterThenItUsesTheTimeAskedAbout(t *testing.T) {
	// Given a fresh mains reading with power safe for 29 of the 30 seconds needed.
	at := time.Unix(1_700_000_000, 0)
	snapshot := Snapshot{
		Reading:          &nut.Reading{At: at, Status: "OL", Battery: 100},
		SafeSince:        at.Add(-29 * time.Second),
		RecoverySettings: recovery.Settings{StablePowerDelay: 30 * time.Second, MaxReadingAge: 30 * time.Second},
	}

	// When the block is checked now and two seconds later.
	now, later := snapshot.PowerBlock(at), snapshot.PowerBlock(at.Add(2*time.Second))

	// Then it changes once the stability delay has passed.
	if now != recovery.Stabilising || later != recovery.NoBlock {
		t.Fatalf("now %q, later %q", now, later)
	}
}
