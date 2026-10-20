import type { Status } from '../../api';
import { DeviceCard } from './DeviceCard';

export function DeviceList({
  status,
  stale,
}: {
  status: Status;
  stale: boolean;
}) {
  const countingFrom = !stale && !status.power.block ? status.checked_at : null;

  return (
    <section aria-labelledby="devices-heading" className="flex flex-col gap-4">
      <h2 id="devices-heading" className="eyebrow">
        Devices
      </h2>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {status.devices.map((device) => (
          <DeviceCard
            key={device.id}
            device={device}
            stale={stale}
            countingFrom={countingFrom}
          />
        ))}
      </div>
    </section>
  );
}
