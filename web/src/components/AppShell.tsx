import type { ReactNode } from 'react';

export function AppShell({
  children,
  valid,
  version,
}: {
  children: ReactNode;
  valid: boolean;
  version?: string;
}) {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b border-border">
        <div className="mx-auto flex h-20 max-w-[1180px] items-center justify-between px-5 sm:px-9">
          <a
            href="/"
            aria-label="Evigo home"
            className="flex items-center gap-5"
          >
            <span className="brand-wordmark text-[32px] sm:text-[36px]">
              evigo
            </span>
            <span className="hidden border-l border-border pl-5 text-xs text-muted-foreground sm:block">
              UPS recovery
            </span>
          </a>
          <span className="flex items-center gap-2 text-xs text-muted-foreground">
            {valid ? 'Live' : 'Reconnecting'}
            <span
              className={`size-2 rounded-full ${valid ? 'bg-success' : 'bg-warning'}`}
            />
          </span>
        </div>
      </header>
      <main className="mx-auto flex max-w-[1180px] flex-col gap-8 px-5 py-8 sm:px-9 sm:py-10">
        {children}
        <footer className="border-t border-border pt-6 text-xs text-muted-foreground">
          evigo {version}
        </footer>
      </main>
    </div>
  );
}
