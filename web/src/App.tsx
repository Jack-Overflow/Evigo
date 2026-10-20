import { LoaderCircle } from 'lucide-react';
import type { Status } from './api';
import { useStatus } from './hooks/useStatus';
import { AppShell } from './components/AppShell';
import { Notice } from './components/Notice';
import { PowerPanel } from './components/power/PowerPanel';
import { DeviceList } from './components/devices/DeviceList';
import { HistoryList } from './components/history/HistoryList';

export default function App() {
  const { status, unreachable, stale } = useStatus();
  const notice = connectionNotice(unreachable, status);

  return (
    <AppShell valid={!stale} version={status?.version}>
      {notice && <Notice>{notice}</Notice>}
      {status && !status.storage_available && (
        <Notice>
          Evigo cannot save its recovery progress, so waking is paused until it
          can.
        </Notice>
      )}
      {!status && !unreachable && (
        <p className="flex items-center gap-3 py-20 text-muted-foreground">
          <LoaderCircle className="animate-spin" size={20} />
          Connecting to Evigo…
        </p>
      )}
      {status && (
        <>
          <PowerPanel status={status} stale={stale} />
          <DeviceList status={status} stale={stale} />
          <HistoryList devices={status.devices} />
        </>
      )}
    </AppShell>
  );
}

function connectionNotice(
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
