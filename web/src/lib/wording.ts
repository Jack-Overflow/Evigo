import type { Power, PowerBlock, Reachability, Settings, Status } from '../api';

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

export function powerDetail(
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

export function ageText(seconds: number): string | null {
  if (seconds < -60) {
    return null;
  }

  if (seconds < 5) {
    return 'just now';
  }

  return `${durationText(seconds)} ago`;
}

export function chargeText(percent: number | null): string {
  return percent == null ? 'Unknown charge' : `${Math.round(percent)}% charge`;
}

export function gaugeValueText(
  percent: number | null,
  minimum: number,
  valid: boolean,
): string {
  const lastKnown = valid ? '' : '; last known reading';

  return `${chargeText(percent)}; waking needs ${minimum}%${lastKnown}`;
}

export function gaugeCaption(minimum: number, valid: boolean): string {
  if (!valid) {
    return 'Last known value, not a fresh reading';
  }

  return `Waking needs at least ${minimum}%`;
}

export function connectionNotice(
  unreachable: boolean,
  status: Status | undefined,
): string | null {
  if (unreachable && status) {
    return 'Evigo is not responding. Showing the last known state.';
  }

  if (unreachable) {
    return 'Evigo is not responding.';
  }

  if (status?.monitor === 'stalled') {
    return 'Evigo has stopped checking the UPS and devices. Showing the last known state.';
  }

  if (status?.monitor === 'starting') {
    return 'Evigo is starting and has not checked the UPS and devices yet.';
  }

  return null;
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

export function reachabilityText(
  reachability: Reachability,
  stale: boolean,
): {
  label: string;
  tone: Tone;
} {
  if (stale) {
    return { label: 'Outdated', tone: 'neutral' };
  }

  const texts: Record<Reachability, { label: string; tone: Tone }> = {
    online: { label: 'Online', tone: 'good' },
    offline: { label: 'Not responding', tone: 'warn' },
    unknown: { label: 'Unknown', tone: 'neutral' },
  };

  return texts[reachability];
}
