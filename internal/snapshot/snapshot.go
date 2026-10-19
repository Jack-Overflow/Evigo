package snapshot

import (
	"slices"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/devices"
	"github.com/Jack-Overflow/Evigo/internal/nut"
	"github.com/Jack-Overflow/Evigo/internal/recovery"
)

type Snapshot struct {
	CheckedAt        time.Time
	Reading          *nut.Reading
	SafeSince        time.Time
	StorageOK        bool
	RecoverySettings recovery.Settings
	Devices          []Device
}

type Device struct {
	Device       devices.Device
	Reachability devices.Reachability
	Pending      *recovery.Progress
	WakeSent     bool
}

func (snapshot Snapshot) PowerBlock(at time.Time) recovery.Block {
	return snapshot.RecoverySettings.PowerBlock(at, snapshot.Reading, snapshot.SafeSince)
}

func (snapshot Snapshot) Copy() Snapshot {
	if snapshot.Reading != nil {
		snapshot.Reading = snapshot.Reading.Copy()
	}

	snapshot.Devices = slices.Clone(snapshot.Devices)
	for index := range snapshot.Devices {
		device := &snapshot.Devices[index]
		device.Device.MAC = slices.Clone(device.Device.MAC)
		if device.Pending != nil {
			progress := *device.Pending
			device.Pending = &progress
		}
	}

	return snapshot
}
