import type { ReactNode } from 'react';
import type { Power, PowerBlock, Settings, Status } from '../../api';
import { useServerNow } from '../../hooks/useServerNow';
import { durationText, type Tone, upsStatusText } from '../../lib/format';
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
  const now = useServerNow();
  const { power, settings, devices } = status;
  const headline = powerHeadline(power);
  const devicesToWake = devices.filter(
    (device) => device.recovery && device.reachability !== 'online',
  ).length;
  const wakeAllowedInSeconds = power.wake_allowed_at
    ? (new Date(power.wake_allowed_at).getTime() - now) / 1000
    : null;
  const detail = powerDetail(power, settings, wakeAllowedInSeconds, stale);

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
            <Detail label="Recovery">{recoverySummary(devicesToWake)}</Detail>
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

function recoverySummary(devicesToWake: number): string {
  if (devicesToWake === 0) {
    return 'Nothing to wake';
  }

  return devicesToWake === 1
    ? '1 device to wake'
    : `${devicesToWake} devices to wake`;
}

function powerHeadline(power: Power): { title: string; tone: Tone } {
  if (!power.block) {
    return { title: 'Power is OK', tone: 'good' };
  }

  if (power.block === 'unsafe_status' && upsSettling(power.status)) {
    return { title: 'Waiting for the UPS to settle', tone: 'neutral' };
  }

  const headlines: Record<PowerBlock, { title: string; tone: Tone }> = {
    stabilising: { title: 'Checking power is stable', tone: 'neutral' },
    on_battery: { title: 'Running on battery', tone: 'warn' },
    battery_low: { title: 'Battery is recharging', tone: 'warn' },
    unsafe_status: { title: 'UPS reports a problem', tone: 'warn' },
    no_reading: { title: 'Power status unknown', tone: 'warn' },
  };

  return headlines[power.block];
}

function powerDetail(
  power: Power,
  settings: Settings,
  wakeAllowedInSeconds: number | null,
  stale: boolean,
): string {
  if (stale) {
    return 'Showing the last known state until Evigo answers again';
  }

  if (power.block) {
    return blockText(power.block, settings, wakeAllowedInSeconds);
  }

  return 'Mains power is on and stable';
}

function blockText(
  block: PowerBlock,
  settings: Settings,
  wakeAllowedInSeconds: number | null,
): string {
  if (block === 'stabilising' && wakeAllowedInSeconds != null) {
    return wakeAllowedText(wakeAllowedInSeconds);
  }

  const texts: Record<PowerBlock, string> = {
    stabilising: 'Waiting for mains power to stay on long enough',
    on_battery: 'Devices are woken once mains power is back and stable',
    battery_low: `Waiting for the battery to reach ${settings.min_battery_percent}% before waking anything`,
    unsafe_status: 'Waking is paused until the UPS reports normal mains power',
    no_reading: 'Evigo cannot get a fresh UPS reading, so waking is paused',
  };

  return texts[block];
}

function wakeAllowedText(seconds: number): string {
  if (seconds <= 0) {
    return 'Waking is allowed shortly';
  }

  return `Waking is allowed in ${durationText(Math.ceil(seconds))}`;
}

const settlingFlags = ['OL', 'CHRG', 'DISCHRG', 'TRIM', 'BOOST'];

function upsSettling(flags: string[]): boolean {
  return (
    flags.includes('OL') && flags.every((flag) => settlingFlags.includes(flag))
  );
}
