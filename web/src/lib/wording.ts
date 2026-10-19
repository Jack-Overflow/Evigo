import type { Device, Power, PowerBlock, Reachability, Settings } from '../api';

export type Tone = 'good' | 'warn' | 'neutral';

const units: [string, number][] = [
  ['d', 86400],
  ['h', 3600],
  ['m', 60],
];

export function durationText(seconds: number): string {
  const whole = Math.max(0, Math.round(seconds));
  const index = units.findIndex(([, size]) => whole >= size);

  if (index === -1) {
    return `${whole}s`;
  }

  const [label, size] = units[index];
  const [nextLabel, nextSize] = units[index + 1] ?? ['s', 1];
  const remainder = Math.floor((whole % size) / nextSize);
  const largest = `${Math.floor(whole / size)}${label}`;

  return remainder ? `${largest} ${remainder}${nextLabel}` : largest;
}

export function recoverySummary(recovering: number, total: number): string {
  return recovering
    ? `${recovering} of ${total} devices recovering`
    : 'Nothing to recover';
}

export function powerHeadline(power: Power): { title: string; tone: Tone } {
  if (!power.block) {
    return { title: 'Power is OK', tone: 'good' };
  }

  const headlines: Record<PowerBlock, { title: string; tone: Tone }> = {
    stabilising: { title: 'Mains is back, stabilising', tone: 'neutral' },
    on_battery: { title: 'Running on battery', tone: 'warn' },
    battery_low: { title: 'Battery is recharging', tone: 'warn' },
    unsafe_status: { title: 'UPS reports a problem', tone: 'warn' },
    no_reading: { title: 'Power status unknown', tone: 'warn' },
  };

  return headlines[power.block];
}

export function blockText(
  block: PowerBlock,
  settings: Settings,
  wakeAllowedInSeconds: number | null,
): string {
  if (block === 'stabilising' && wakeAllowedInSeconds != null) {
    return `Waking is allowed in ${durationText(wakeAllowedInSeconds)}`;
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

const upsFlags: Record<string, string> = {
  OL: 'On mains',
  OB: 'On battery',
  LB: 'Low battery',
  RB: 'Replace battery',
  CHRG: 'Charging',
  DISCHRG: 'Discharging',
  BYPASS: 'Bypass',
  CAL: 'Calibrating',
  OFF: 'Output off',
  OVER: 'Overloaded',
  TRIM: 'Trimming voltage',
  BOOST: 'Boosting voltage',
  FSD: 'Forced shutdown',
  ALARM: 'Alarm',
  UNKNOWN: 'Unrecognised status',
};

export function upsStatusText(flags: string[]): string {
  if (flags.length === 0) {
    return 'Unknown';
  }

  return flags.map((flag) => upsFlags[flag] ?? flag).join(' · ');
}

export function reachabilityText(reachability: Reachability): {
  label: string;
  tone: Tone;
} {
  const texts: Record<Reachability, { label: string; tone: Tone }> = {
    online: { label: 'Online', tone: 'good' },
    offline: { label: 'Not responding', tone: 'warn' },
    unknown: { label: 'Unknown', tone: 'neutral' },
  };

  return texts[reachability];
}

export function deviceStageText(
  device: Device,
  block: PowerBlock | null,
): string {
  const recovery = device.recovery;

  if (!recovery) {
    const idle: Record<Reachability, string> = {
      online: 'Running',
      offline:
        'Not running. Evigo only wakes devices that were on before an outage.',
      unknown: 'Evigo cannot check this device right now',
    };

    return idle[device.reachability];
  }

  if (block) {
    return 'Recovering. It will be woken once power is ready.';
  }

  if (device.reachability === 'unknown') {
    return 'Recovering, but Evigo cannot check it, so it is not woken yet';
  }

  if (recovery.wake_sent) {
    return 'Wake sent. Waiting for it to come back online.';
  }

  if (device.reachability === 'online') {
    return 'Still answering. Giving it time to finish shutting down.';
  }

  return 'Off after the outage. It is being woken.';
}
