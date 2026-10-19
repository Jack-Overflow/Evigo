import { useServerNow } from '../hooks/useServerNow';
import { ageText } from '../lib/wording';
import { Tooltip } from './ui/tooltip';

const fullDate = new Intl.DateTimeFormat('en-GB', {
  day: '2-digit',
  month: 'short',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
  timeZoneName: 'shortOffset',
});

export function Timestamp({
  value,
  prefix,
  empty = 'Not yet',
}: {
  value: string | null | undefined;
  prefix?: string;
  empty?: string;
}) {
  const now = useServerNow();
  const date = value ? new Date(value) : undefined;

  if (!value || !date || !Number.isFinite(date.getTime())) {
    return <span>{empty}</span>;
  }

  const full = fullDate.format(date);
  const age = [prefix, ageText((now - date.getTime()) / 1000) ?? full]
    .filter(Boolean)
    .join(' ');

  return (
    <Tooltip content={full}>
      <time dateTime={value} tabIndex={0} aria-label={`${age}, ${full}`}>
        {age}
      </time>
    </Tooltip>
  );
}
