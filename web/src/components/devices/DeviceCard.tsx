import type { ReactNode } from 'react';
import { Server } from 'lucide-react';
import type { Device } from '../../api';
import { durationText, reachabilityText, type Tone } from '../../lib/format';
import { Timestamp } from '../Timestamp';
import { Card } from '../ui/card';

const badgeColours: Record<Tone, string> = {
  good: 'border-success/30 text-success',
  warn: 'border-warning/30 text-warning',
  neutral: 'border-border text-muted-foreground',
};

export function DeviceCard({
  device,
  stale,
}: {
  device: Device;
  stale: boolean;
}) {
  const reachability = reachabilityText(device.reachability, stale);
  const recovery = device.recovery;

  return (
    <Card className="flex flex-col gap-4 p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <div className="shrink-0 rounded-lg border border-border bg-muted/50 p-2 text-muted-foreground">
            <Server size={18} strokeWidth={1.5} />
          </div>
          <div className="min-w-0">
            <h3 className="truncate font-semibold tracking-tight">
              {device.name}
            </h3>
            <p className="truncate font-mono text-xs text-muted-foreground">
              {device.host}
            </p>
          </div>
        </div>
        <span
          className={`shrink-0 rounded-full border px-2.5 py-1 text-xs ${badgeColours[reachability.tone]}`}
        >
          {reachability.label}
        </span>
      </div>
      <dl className="flex flex-col gap-2.5 text-xs text-muted-foreground">
        <Detail label="MAC address">
          <span className="font-mono">{device.mac}</span>
        </Detail>
        {recovery && (
          <>
            <Detail label="Wake attempts">{recovery.attempts}</Detail>
            <Detail label="Last attempt">
              <Timestamp value={recovery.last_attempt_at} empty="None yet" />
            </Detail>
            <Detail label="Time left to recover">
              {durationText(recovery.time_left_seconds)}
            </Detail>
            {!recovery.wake_sent && recovery.grace_left_seconds > 0 && (
              <Detail label="Shutdown grace left">
                {durationText(recovery.grace_left_seconds)}
              </Detail>
            )}
          </>
        )}
      </dl>
      {recovery && (
        <p className="text-xs text-muted-foreground">
          Time left only counts down while waking is allowed.
        </p>
      )}
    </Card>
  );
}

function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex justify-between gap-3">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}
