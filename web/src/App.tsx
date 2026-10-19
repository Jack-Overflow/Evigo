import { LoaderCircle } from 'lucide-react';
import { useStatus } from './hooks/useStatus';
import { AppShell } from './components/AppShell';
import { Notice } from './components/Notice';
import { PowerPanel } from './components/power/PowerPanel';
import { DeviceList } from './components/devices/DeviceList';

export default function App() {
  const { status, unreachable, stale } = useStatus();

  return (
    <AppShell valid={!stale} version={status?.version}>
      {unreachable && (
        <Notice>
          Evigo is not responding.
          {status && ' Showing the last known state.'}
        </Notice>
      )}
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
        </>
      )}
    </AppShell>
  );
}
