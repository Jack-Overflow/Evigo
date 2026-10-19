import type { Status } from '../../api';
import { DeviceCard } from './DeviceCard';

export function DeviceList({
  status,
  stale,
}: {
  status: Status;
  stale: boolean;
}) {
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
            block={status.power.block}
            stale={stale}
          />
        ))}
      </div>
    </section>
  );
}
