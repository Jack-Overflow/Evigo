import type { ReactNode } from 'react';
import type { Status } from '../../api';
import { useNow } from '../../hooks/useNow';
import {
  blockText,
  powerHeadline,
  recoverySummary,
  upsStatusText,
} from '../../lib/wording';
import { Timestamp } from '../Timestamp';
import { Card } from '../ui/card';
import { BatteryGauge } from './BatteryGauge';

const toneColours = {
  good: 'text-success',
  warn: 'text-warning',
  neutral: 'text-foreground',
};

export function PowerPanel({
  status,
  stale,
}: {
  status: Status;
  stale: boolean;
}) {
  const now = useNow();
  const { power, settings, devices } = status;
  const headline = powerHeadline(power);
  const recovering = devices.filter((device) => device.recovery).length;
  const wakeAllowedInSeconds = power.wake_allowed_at
    ? Math.max(0, (new Date(power.wake_allowed_at).getTime() - now) / 1000)
    : null;
  let detail = power.block
    ? blockText(power.block, settings, wakeAllowedInSeconds)
    : 'Mains power is on and stable';

  if (stale) {
    detail = 'Showing the last known state until Evigo answers again';
  }

  return (
    <section aria-labelledby="power-heading">
      <Card className="grid gap-0 overflow-hidden p-0 lg:grid-cols-[1fr_340px]">
        <div className="flex flex-col gap-6 p-6 sm:p-7">
          <div>
            <h2 id="power-heading" className="eyebrow">
              Power
            </h2>
            <p
              className={`mt-3 text-3xl font-semibold tracking-tight ${stale ? 'text-muted-foreground' : toneColours[headline.tone]}`}
            >
              {headline.title}
            </p>
            <p className="mt-2 text-sm text-muted-foreground">{detail}</p>
          </div>
          <dl className="grid gap-x-8 gap-y-4 text-sm sm:grid-cols-2">
            <Detail label="UPS">{power.ups}</Detail>
            <Detail label="Status">{upsStatusText(power.status)}</Detail>
            <Detail label="Last reading">
              <Timestamp value={power.read_at} empty="No reading yet" />
            </Detail>
            <Detail label="Recovery">
              {recoverySummary(recovering, devices.length)}
            </Detail>
          </dl>
        </div>
        <BatteryGauge
          percent={power.battery_percent}
          minimum={settings.min_battery_percent}
          valid={power.available && !stale}
          charging={power.status.includes('CHRG')}
        />
      </Card>
    </section>
  );
}

function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 truncate">{children}</dd>
    </div>
  );
}
