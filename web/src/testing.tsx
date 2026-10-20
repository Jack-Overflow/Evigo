import { render } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { vi } from 'vitest';
import App from './App';
import type { Device, DeviceRecovery, Power, Status } from './api';

type StatusOverrides = Partial<Omit<Status, 'power'>> & {
  power?: Partial<Power>;
};

export function statusWith({
  power,
  ...overrides
}: StatusOverrides = {}): Status {
  return {
    version: '0.2.0',
    server_time: new Date().toISOString(),
    checked_at: new Date().toISOString(),
    monitor: 'running',
    power: {
      ups: 'main-ups',
      available: true,
      status: ['OL'],
      battery_percent: 100,
      read_at: new Date().toISOString(),
      on_battery: false,
      charging: false,
      block: null,
      stable_since: new Date(Date.now() - 60_000).toISOString(),
      wake_allowed_at: new Date(Date.now() - 30_000).toISOString(),
      ...power,
    },
    storage_available: true,
    settings: {
      min_battery_percent: 25,
      stable_power_seconds: 30,
      retry_seconds: 30,
      device_timeout_seconds: 600,
    },
    devices: [
      {
        id: 'nas',
        name: 'NAS',
        host: '192.0.2.10',
        mac: '02:00:00:00:00:10',
        reachability: 'online',
        recovery: null,
      },
    ],
    ...overrides,
  };
}

export function stubStatusFetch(
  statusFetch: (url: string, init: RequestInit) => Promise<Response>,
) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      if (url.startsWith('/api/history')) {
        return Response.json({ events: [], next_before: null });
      }

      return statusFetch(url, init);
    }),
  );
}

export function nas(
  reachability: Device['reachability'],
  recovery: DeviceRecovery | null = null,
): Device {
  return {
    id: 'nas',
    name: 'NAS',
    host: '192.0.2.10',
    mac: '02:00:00:00:00:10',
    reachability,
    recovery,
  };
}

export function recoveryOf(
  overrides: Partial<DeviceRecovery> = {},
): DeviceRecovery {
  return {
    state: 'waking',
    attempts: 0,
    last_attempt_at: null,
    time_left_seconds: 600,
    gives_up_at: null,
    ...overrides,
  };
}

export function answerAtTheCurrentTime(status: Status) {
  stubStatusFetch(async () =>
    Response.json({ ...status, server_time: new Date().toISOString() }),
  );
}

export function answerWith(status: Status) {
  stubStatusFetch(async () => Response.json(status));
}

export function timeOutWithFakeTimers() {
  vi.spyOn(AbortSignal, 'timeout').mockImplementation((milliseconds) => {
    const controller = new AbortController();
    setTimeout(
      () => controller.abort(new DOMException('Timed out', 'TimeoutError')),
      milliseconds,
    );

    return controller.signal;
  });
}

export function renderApp() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );
}
