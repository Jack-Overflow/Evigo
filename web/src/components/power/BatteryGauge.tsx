import { gaugeCaption, gaugeValueText } from '../../lib/wording';

const segments = 20;
const segmentGapPx = 6;

export function BatteryGauge({
  percent,
  minimum,
  valid,
  charging,
}: {
  percent: number | null;
  minimum: number;
  valid: boolean;
  charging: boolean;
}) {
  const filledSegments =
    percent == null ? 0 : Math.ceil(percent / (100 / segments));
  const reserveSegments = Math.ceil(minimum / (100 / segments));
  const belowMinimum = valid && percent != null && percent < minimum;

  function segmentColour(index: number) {
    if (index >= filledSegments) {
      return index < reserveSegments ? 'bg-warning/10' : 'bg-muted/50';
    }

    if (!valid) {
      return 'bg-muted';
    }

    return belowMinimum ? 'bg-warning' : 'bg-success';
  }

  return (
    <div className="flex flex-col gap-4 border-t border-border bg-muted/30 p-6 lg:border-l lg:border-t-0">
      <div className="flex items-center justify-between">
        <p className="eyebrow">Battery</p>
        {valid && charging && (
          <span className="text-xs text-muted-foreground">Charging</span>
        )}
      </div>
      <p
        className={`text-[40px] font-medium leading-none tracking-tight ${valid ? '' : 'text-muted-foreground'}`}
      >
        {percent == null ? '—' : Math.round(percent)}
        {percent != null && (
          <span className="ml-1 text-lg text-muted-foreground">%</span>
        )}
      </p>
      <div>
        <div
          role="progressbar"
          aria-label="Battery charge"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={percent ?? undefined}
          aria-valuetext={gaugeValueText(percent, minimum, valid)}
          className="flex"
          style={{ gap: segmentGapPx }}
        >
          {Array.from({ length: segments }, (_, index) => (
            <span
              key={index}
              className={`h-3 flex-1 rounded-sm ${segmentColour(index)}`}
            />
          ))}
        </div>
        {minimum > 0 && <ThresholdMarker minimum={minimum} />}
      </div>
      <p className="text-xs text-muted-foreground">
        {gaugeCaption(minimum, valid)}
      </p>
    </div>
  );
}

function ThresholdMarker({ minimum }: { minimum: number }) {
  const fraction = Math.min(Math.max(minimum, 0), 100) / 100;
  const left = `calc(${fraction} * (100% + ${segmentGapPx}px) - ${segmentGapPx / 2}px)`;
  let alignment = '-translate-x-1/2';

  if (fraction < 0.1) {
    alignment = 'translate-x-0';
  } else if (fraction > 0.9) {
    alignment = '-translate-x-full';
  }

  return (
    <div aria-hidden="true" className="relative mt-1.5 h-5">
      <span
        className="absolute top-0 h-1.5 w-px bg-warning/70"
        style={{ left }}
      />
      <span
        className={`absolute top-2 whitespace-nowrap text-[11px] leading-none text-warning/80 ${alignment}`}
        style={{ left }}
      >
        {minimum}% min
      </span>
    </div>
  );
}
