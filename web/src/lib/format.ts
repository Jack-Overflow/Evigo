import type { Reachability } from '../api';

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

export function chargeText(percent: number | null): string {
  return percent == null ? 'Unknown charge' : `${Math.round(percent)}% charge`;
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
