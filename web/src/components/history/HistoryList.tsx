import { useState } from 'react';
import {
  Activity,
  PlugZap,
  Power,
  Server,
  ShieldCheck,
  type LucideIcon,
} from 'lucide-react';
import type { Device, HistoryEvent, HistoryKind } from '../../api';
import { useHistory } from '../../hooks/useHistory';
import { chargeText, reachabilityText, upsStatusText } from '../../lib/format';
import { Timestamp } from '../Timestamp';
import { Button } from '../ui/button';
import { Card } from '../ui/card';

const icons: Record<HistoryKind, LucideIcon> = {
  started: Activity,
  ups_status: PlugZap,
  reachability: Server,
  device_watched: ShieldCheck,
  device_recovered: ShieldCheck,
  device_already_on: ShieldCheck,
  device_gave_up: ShieldCheck,
  wake_sent: Power,
  wake_failed: Power,
};

export function HistoryList({ devices }: { devices: Device[] }) {
  const [cursors, setCursors] = useState<number[]>([]);
  const { page, unavailable } = useHistory(cursors.at(-1));
  const deviceNames = new Map(
    devices.map((device) => [device.id, device.name]),
  );
  const nextBefore = page?.next_before;

  return (
    <section aria-labelledby="history-heading" className="flex flex-col gap-4">
      <h2 id="history-heading" className="eyebrow">
        History
      </h2>
      <Card className="gap-0 p-0">
        {!page && unavailable && (
          <p className="p-5 text-sm text-muted-foreground">
            History is unavailable
          </p>
        )}
        {page?.events.length === 0 && (
          <p className="p-5 text-sm text-muted-foreground">
            Nothing recorded yet
          </p>
        )}
        {page && page.events.length > 0 && (
          <ol className="divide-y divide-border">
            {page.events.map((event) => {
              const Icon = icons[event.kind];
              const problem = historyIsProblem(event);
              const deviceName =
                deviceNames.get(event.device_id ?? '') ?? event.device_id ?? '';

              return (
                <li
                  key={event.id}
                  className="flex items-center gap-4 px-5 py-3 text-sm"
                >
                  <span
                    className={`rounded-full border border-border p-2 ${problem ? 'text-warning' : 'text-muted-foreground'}`}
                  >
                    <Icon size={14} aria-hidden="true" />
                  </span>
                  <div className="flex min-w-0 flex-1 flex-wrap items-center justify-between gap-x-4 gap-y-1">
                    <span className={problem ? 'text-warning' : ''}>
                      {historyText(event, deviceName)}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      <Timestamp value={event.at} />
                    </span>
                  </div>
                </li>
              );
            })}
          </ol>
        )}
      </Card>
      {(cursors.length > 0 || nextBefore != null) && (
        <div className="flex justify-between gap-3">
          <Button
            variant="outline"
            className="text-xs"
            disabled={cursors.length === 0}
            onClick={() => setCursors((previous) => previous.slice(0, -1))}
          >
            Newer
          </Button>
          <Button
            variant="outline"
            className="text-xs"
            disabled={nextBefore == null}
            onClick={() =>
              nextBefore != null &&
              setCursors((previous) => [...previous, nextBefore])
            }
          >
            Older
          </Button>
        </div>
      )}
    </section>
  );
}

function historyText(event: HistoryEvent, deviceName: string): string {
  const { version, status, battery_percent, reachability, attempt } =
    event.details;

  switch (event.kind) {
    case 'started':
      return `Evigo ${version} started`;
    case 'ups_status':
      return upsHistoryText(status, battery_percent);
    case 'reachability':
      return `${deviceName} is ${reachabilityText(reachability ?? 'unknown', false).label.toLowerCase()}`;
    case 'device_watched':
      return `Watching ${deviceName} until power is stable`;
    case 'wake_sent':
      return `Wake sent to ${deviceName} (attempt ${attempt})`;
    case 'wake_failed':
      return `Wake to ${deviceName} failed (attempt ${attempt})`;
    case 'device_recovered':
      return `${deviceName} recovered`;
    case 'device_already_on':
      return `${deviceName} was already on, no wake needed`;
    case 'device_gave_up':
      return `Gave up on ${deviceName} after ${attempt} ${attempt === 1 ? 'attempt' : 'attempts'}`;
  }
}

function upsHistoryText(
  status: string | undefined,
  batteryPercent: number | undefined,
): string {
  if (!status) {
    return 'UPS: no reading';
  }

  const flags = upsStatusText(status.split(' '));

  if (batteryPercent == null) {
    return `UPS: ${flags}`;
  }

  return `UPS: ${flags} · ${chargeText(batteryPercent)}`;
}

function historyIsProblem(event: HistoryEvent): boolean {
  const { status, on_battery, reachability } = event.details;

  switch (event.kind) {
    case 'ups_status':
      return !status || Boolean(on_battery);
    case 'reachability':
      return reachability !== 'online';
    case 'wake_failed':
    case 'device_gave_up':
      return true;
    default:
      return false;
  }
}
