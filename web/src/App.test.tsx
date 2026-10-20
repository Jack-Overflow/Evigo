import { act, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App';
import type { Device, DeviceRecovery, Status } from './api';

function statusWith(overrides: Partial<Status> = {}): Status {
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
      block: null,
      stable_since: new Date(Date.now() - 60_000).toISOString(),
      wake_allowed_at: new Date(Date.now() - 30_000).toISOString(),
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

function stubStatusFetch(
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

function nasWith(
  reachability: Device['reachability'],
  recovery: DeviceRecovery,
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

const woken: DeviceRecovery = {
  attempts: 1,
  last_attempt_at: '2026-01-01T12:00:00Z',
  time_left_seconds: 540,
};

function answerAtTheCurrentTime(status: Status) {
  stubStatusFetch(async () =>
    Response.json({ ...status, server_time: new Date().toISOString() }),
  );
}

function answerWith(status: Status) {
  stubStatusFetch(async () => Response.json(status));
}

function timeOutWithFakeTimers() {
  vi.spyOn(AbortSignal, 'timeout').mockImplementation((milliseconds) => {
    const controller = new AbortController();
    setTimeout(
      () => controller.abort(new DOMException('Timed out', 'TimeoutError')),
      milliseconds,
    );

    return controller.signal;
  });
}

function renderApp() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("Given a request for Evigo's status", () => {
  describe('When power is on and stable', () => {
    beforeEach(() => {
      answerWith(statusWith());
      renderApp();
    });

    it('Then it says power is OK', async () => {
      expect(await screen.findByText('Power is OK')).toBeInTheDocument();
    });

    it('Then it shows the battery against the minimum', async () => {
      expect(
        await screen.findByRole('progressbar', { name: 'Battery charge' }),
      ).toHaveAttribute('aria-valuetext', '100% charge; waking needs 25%');
    });

    it('Then it shows each device and that it is online', async () => {
      expect(await screen.findByText('NAS')).toBeInTheDocument();
      expect(screen.getByText('Online')).toBeInTheDocument();
    });

    it('Then it shows the version', async () => {
      expect(await screen.findByText('evigo 0.2.0')).toBeInTheDocument();
    });
  });

  describe('When power has just come back', () => {
    beforeEach(() => {
      vi.useFakeTimers({ toFake: ['Date'] });
      vi.setSystemTime(new Date('2026-01-01T12:00:00Z'));
      const status = statusWith({ server_time: '2026-01-01T12:00:00Z' });
      status.power.block = 'stabilising';
      status.power.wake_allowed_at = '2026-01-01T12:00:20Z';
      answerWith(status);
      renderApp();
    });

    it('Then it says it is checking power is stable', async () => {
      expect(
        await screen.findByText('Checking power is stable'),
      ).toBeInTheDocument();
    });

    it('Then it counts down to when waking is allowed', async () => {
      expect(
        await screen.findByText('Waking is allowed in 20s'),
      ).toBeInTheDocument();
    });
  });

  describe('When the battery is below the minimum', () => {
    beforeEach(() => {
      const status = statusWith();
      status.power.block = 'battery_low';
      status.power.battery_percent = 10;
      answerWith(status);
      renderApp();
    });

    it('Then it says waking waits for the minimum charge', async () => {
      expect(
        await screen.findByText(
          'Waiting for the battery to reach 25% before waking anything',
        ),
      ).toBeInTheDocument();
    });
  });

  describe('When a device is recovering after an outage', () => {
    beforeEach(() => {
      answerWith(
        statusWith({
          devices: [
            {
              id: 'nas',
              name: 'NAS',
              host: '192.0.2.10',
              mac: '02:00:00:00:00:10',
              reachability: 'offline',
              recovery: {
                attempts: 2,
                last_attempt_at: new Date().toISOString(),
                time_left_seconds: 540,
              },
            },
          ],
        }),
      );
      renderApp();
    });

    it('Then it shows the wake attempts', async () => {
      expect(await screen.findByText('Wake attempts')).toBeInTheDocument();
      expect(screen.getByText('Wake attempts').nextSibling).toHaveTextContent(
        '2',
      );
    });

    it('Then it shows the time left to recover', async () => {
      expect(
        await screen.findByText('Time left to recover'),
      ).toBeInTheDocument();
      expect(
        screen.getByText('Time left to recover').nextSibling,
      ).toHaveTextContent('9m');
    });
  });

  describe('When a device that stayed on is watched during an outage', () => {
    beforeEach(() => {
      const status = statusWith({
        devices: [
          nasWith('online', {
            attempts: 0,
            last_attempt_at: null,
            time_left_seconds: 600,
          }),
        ],
      });
      status.power.block = 'on_battery';
      answerWith(status);
      renderApp();
    });

    it('Then its card shows no recovery details', async () => {
      expect(await screen.findByText('NAS')).toBeInTheDocument();
      expect(
        screen.queryByText('Time left to recover'),
      ).not.toBeInTheDocument();
    });

    it('Then the summary has nothing to wake', async () => {
      expect(await screen.findByText('Nothing to wake')).toBeInTheDocument();
    });
  });

  describe('When a device is off during an outage', () => {
    beforeEach(() => {
      const status = statusWith({
        devices: [
          nasWith('offline', {
            attempts: 0,
            last_attempt_at: null,
            time_left_seconds: 600,
          }),
        ],
      });
      status.power.block = 'on_battery';
      answerWith(status);
      renderApp();
    });

    it('Then its card shows no time left or attempt details', async () => {
      expect(await screen.findByText('Offline')).toBeInTheDocument();
      expect(
        screen.queryByText('Time left to recover'),
      ).not.toBeInTheDocument();
      expect(screen.queryByText('Wake attempts')).not.toBeInTheDocument();
    });

    it('Then the summary counts it as a device to wake', async () => {
      expect(await screen.findByText('1 device to wake')).toBeInTheDocument();
    });
  });

  describe('When time passes between checks while waking is allowed', () => {
    beforeEach(async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      vi.setSystemTime(new Date('2026-01-01T12:00:00Z'));
      answerAtTheCurrentTime(
        statusWith({
          checked_at: '2026-01-01T12:00:00Z',
          devices: [nasWith('offline', woken)],
        }),
      );
      renderApp();
      await screen.findByText('Time left to recover');
      await act(() => vi.advanceTimersByTimeAsync(30_000));
    });

    it('Then the time left counts down', async () => {
      expect(
        screen.getByText('Time left to recover').nextSibling,
      ).toHaveTextContent('8m 30s');
    });
  });

  describe('When waking became allowed after Evigo last checked', () => {
    beforeEach(async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      vi.setSystemTime(new Date('2026-01-01T12:00:10Z'));
      const status = statusWith({
        checked_at: '2026-01-01T12:00:00Z',
        devices: [nasWith('offline', woken)],
      });
      status.power.wake_allowed_at = '2026-01-01T12:00:05Z';
      answerAtTheCurrentTime(status);
      renderApp();
      await screen.findByText('Wake attempts');
      await act(() => vi.advanceTimersByTimeAsync(3_000));
    });

    it('Then it does not count down time Evigo has not started spending', () => {
      expect(
        screen.queryByText('Time left to recover'),
      ).not.toBeInTheDocument();
    });
  });

  describe('When waking is paused after a wake attempt', () => {
    beforeEach(() => {
      const status = statusWith({ devices: [nasWith('offline', woken)] });
      status.power.block = 'on_battery';
      answerWith(status);
      renderApp();
    });

    it('Then it keeps showing the wake attempts', async () => {
      expect(
        (await screen.findByText('Wake attempts')).nextSibling,
      ).toHaveTextContent('1');
    });

    it('Then it hides the time left while it is not counting down', async () => {
      await screen.findByText('Wake attempts');

      expect(
        screen.queryByText('Time left to recover'),
      ).not.toBeInTheDocument();
    });
  });

  describe('When the UPS briefly reports discharging after mains returns', () => {
    beforeEach(() => {
      const status = statusWith();
      status.power.block = 'unsafe_status';
      status.power.status = ['OL', 'CHRG', 'DISCHRG'];
      answerWith(status);
      renderApp();
    });

    it('Then it says it is waiting for the UPS to settle', async () => {
      expect(
        await screen.findByText('Waiting for the UPS to settle'),
      ).toBeInTheDocument();
    });
  });

  describe('When the UPS reports a fault', () => {
    beforeEach(() => {
      const status = statusWith();
      status.power.block = 'unsafe_status';
      status.power.status = ['OL', 'OVER'];
      answerWith(status);
      renderApp();
    });

    it('Then it says the UPS reports a problem', async () => {
      expect(
        await screen.findByText('UPS reports a problem'),
      ).toBeInTheDocument();
    });
  });

  describe('When Evigo cannot save its progress', () => {
    beforeEach(() => {
      answerWith(statusWith({ storage_available: false }));
      renderApp();
    });

    it('Then it warns that waking is paused', async () => {
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Evigo cannot save its recovery progress, so waking is paused until it can.',
      );
    });
  });

  describe('When Evigo cannot be reached', () => {
    beforeEach(() => {
      vi.stubGlobal(
        'fetch',
        vi.fn(async () => {
          throw new TypeError('Failed to fetch');
        }),
      );
      renderApp();
    });

    it('Then it says Evigo is not responding', async () => {
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Evigo is not responding.',
      );
    });
  });

  describe('When Evigo stops answering after a successful update', () => {
    beforeEach(async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      const fetch = vi
        .fn()
        .mockResolvedValueOnce(Response.json(statusWith()))
        .mockRejectedValue(new TypeError('Failed to fetch'));
      stubStatusFetch(fetch);
      renderApp();
      await screen.findByText('Power is OK');
      await act(() => vi.advanceTimersByTimeAsync(6000));
    });

    it('Then it keeps the last known state and says so', async () => {
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Evigo is not responding. Showing the last known state.',
      );
      expect(screen.getByText('NAS')).toBeInTheDocument();
    });

    it('Then devices are marked as outdated', async () => {
      expect(await screen.findByText('Outdated')).toBeInTheDocument();
    });

    it('Then the battery is shown as the last known reading', async () => {
      expect(
        await screen.findByText('Last known value, not a fresh reading'),
      ).toBeInTheDocument();
    });

    it('Then the header shows it is reconnecting', async () => {
      expect(await screen.findByText('Reconnecting')).toBeInTheDocument();
    });
  });

  describe("When the browser's clock is behind Evigo's", () => {
    beforeEach(() => {
      vi.useFakeTimers({ toFake: ['Date'] });
      vi.setSystemTime(new Date('2026-01-01T11:55:00Z'));
      const status = statusWith({
        server_time: '2026-01-01T12:00:00Z',
        checked_at: '2026-01-01T12:00:00Z',
      });
      status.power.block = 'stabilising';
      status.power.wake_allowed_at = '2026-01-01T12:00:20Z';
      answerWith(status);
      renderApp();
    });

    it("Then the countdown uses Evigo's time", async () => {
      expect(
        await screen.findByText('Waking is allowed in 20s'),
      ).toBeInTheDocument();
    });
  });

  describe('When the wake time has already passed', () => {
    beforeEach(() => {
      const status = statusWith();
      status.power.block = 'stabilising';
      status.power.wake_allowed_at = new Date(Date.now() - 2000).toISOString();
      answerWith(status);
      renderApp();
    });

    it('Then it says waking is allowed shortly', async () => {
      expect(
        await screen.findByText('Waking is allowed shortly'),
      ).toBeInTheDocument();
    });
  });

  describe("When Evigo's monitor has stalled", () => {
    beforeEach(() => {
      answerWith(statusWith({ monitor: 'stalled' }));
      renderApp();
    });

    it('Then it says checks have stopped', async () => {
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Evigo has stopped checking the UPS and devices. Showing the last known state.',
      );
    });

    it('Then devices are marked as outdated', async () => {
      expect(await screen.findByText('Outdated')).toBeInTheDocument();
    });
  });

  describe('When Evigo has not finished its first check', () => {
    beforeEach(() => {
      answerWith(statusWith({ monitor: 'starting' }));
      renderApp();
    });

    it('Then it says Evigo is starting', async () => {
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Evigo is starting and has not checked the UPS and devices yet.',
      );
    });
  });

  describe('When a request hangs and a later one succeeds', () => {
    beforeEach(async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      timeOutWithFakeTimers();
      const fetch = vi
        .fn()
        .mockResolvedValueOnce(Response.json(statusWith()))
        .mockImplementationOnce(
          (_url: string, init: RequestInit) =>
            new Promise((_resolve, reject) => {
              init.signal?.addEventListener('abort', () =>
                reject(init.signal?.reason),
              );
            }),
        )
        .mockImplementation(async () => Response.json(statusWith()));
      stubStatusFetch(fetch);
      renderApp();
      await screen.findByText('Power is OK');
      await act(() => vi.advanceTimersByTimeAsync(16000));
    });

    it('Then the hung request times out and it shows as not responding', async () => {
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Evigo is not responding. Showing the last known state.',
      );
    });

    it('Then the next successful update clears the notice', async () => {
      await act(() => vi.advanceTimersByTimeAsync(6000));
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();
      expect(screen.getByText('Live')).toBeInTheDocument();
    });
  });

  describe('When the headers arrive but the body hangs', () => {
    beforeEach(async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      timeOutWithFakeTimers();
      const fetch = vi
        .fn()
        .mockResolvedValueOnce(Response.json(statusWith()))
        .mockImplementation(
          async (_url: string, init: RequestInit) =>
            new Response(
              new ReadableStream({
                start(controller) {
                  init.signal?.addEventListener('abort', () =>
                    controller.error(init.signal?.reason),
                  );
                },
              }),
              { headers: { 'Content-Type': 'application/json' } },
            ),
        );
      stubStatusFetch(fetch);
      renderApp();
      await screen.findByText('Power is OK');
      await act(() => vi.advanceTimersByTimeAsync(16000));
    });

    it('Then it says Evigo is not responding', async () => {
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Evigo is not responding. Showing the last known state.',
      );
    });

    it('Then it keeps the last known state', async () => {
      expect(await screen.findByText('Power is OK')).toBeInTheDocument();
      expect(screen.getByText('NAS')).toBeInTheDocument();
    });
  });

  describe('When a successful response is not JSON', () => {
    beforeEach(async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      const fetch = vi
        .fn()
        .mockResolvedValueOnce(Response.json(statusWith()))
        .mockImplementation(
          async () =>
            new Response('<html>Sign in</html>', {
              headers: { 'Content-Type': 'text/html' },
            }),
        );
      stubStatusFetch(fetch);
      renderApp();
      await screen.findByText('Power is OK');
      await act(() => vi.advanceTimersByTimeAsync(6000));
    });

    it('Then it says Evigo is not responding', async () => {
      expect(await screen.findByRole('alert')).toHaveTextContent(
        'Evigo is not responding. Showing the last known state.',
      );
    });

    it('Then it keeps the last known state', async () => {
      expect(await screen.findByText('Power is OK')).toBeInTheDocument();
      expect(screen.getByText('NAS')).toBeInTheDocument();
    });
  });
});
