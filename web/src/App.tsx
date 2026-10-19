import { LoaderCircle } from 'lucide-react';
import { useStatus } from './hooks/useStatus';

export default function App() {
  const { status, unreachable } = useStatus();

  return (
    <div className="mx-auto flex min-h-screen max-w-5xl flex-col gap-8 px-4 py-8 sm:px-6">
      <header className="flex items-baseline justify-between gap-4">
        <h1 className="brand-wordmark text-3xl">evigo</h1>
        {status && (
          <span className="text-sm text-muted-foreground">
            v{status.version}
          </span>
        )}
      </header>
      <main>
        {unreachable && (
          <p role="alert" className="text-warning">
            Evigo is not responding.
          </p>
        )}
        {!status && !unreachable && (
          <p className="flex items-center gap-3 text-muted-foreground">
            <LoaderCircle className="animate-spin" size={20} />
            Connecting to Evigo…
          </p>
        )}
        {status && (
          <p className="text-muted-foreground">
            Watching {status.power.ups} and {status.devices.length} machines.
          </p>
        )}
      </main>
    </div>
  );
}
