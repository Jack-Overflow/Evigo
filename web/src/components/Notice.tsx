import type { ReactNode } from 'react';
import { Alert, AlertDescription } from './ui/alert';

export function Notice({ children }: { children: ReactNode }) {
  return (
    <Alert
      role="alert"
      className="rounded-xl border border-warning/25 bg-warning/5 p-4 text-sm text-warning"
    >
      <AlertDescription className="text-inherit">{children}</AlertDescription>
    </Alert>
  );
}
