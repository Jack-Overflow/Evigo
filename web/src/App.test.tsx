import { act, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App';
import type { Status } from './api';

function statusWith(overrides: Partial<Status> = {}): Status {
  return {
    version: '0.2.0',
    checked_at: new Date().toISOString(),
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
      shutdown_grace_seconds: 120,
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

function answerWith(status: Status) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => Response.json(status)),
  );
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

    it('Then it shows each device and that it is running', async () => {
      expect(await screen.findByText('NAS')).toBeInTheDocument();
      expect(screen.getByText('Running')).toBeInTheDocument();
    });

    it('Then it shows the version', async () => {
      expect(await screen.findByText('evigo 0.2.0')).toBeInTheDocument();
    });
  });

  describe('When power has just come back', () => {
    beforeEach(() => {
      vi.useFakeTimers({ toFake: ['Date'] });
      vi.setSystemTime(new Date('2026-01-01T12:00:00Z'));
      const status = statusWith();
      status.power.block = 'stabilising';
      status.power.wake_allowed_at = '2026-01-01T12:00:20Z';
      answerWith(status);
      renderApp();
    });

    it('Then it counts down to when waking is allowed', async () => {
      expect(
        await screen.findByText('Waking is allowed in 20s'),
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
                grace_left_seconds: 0,
                wake_sent: true,
              },
            },
          ],
        }),
      );
      renderApp();
    });

    it('Then it says a wake was sent', async () => {
      expect(
        await screen.findByText(
          'Wake sent. Waiting for it to come back online.',
        ),
      ).toBeInTheDocument();
    });

    it('Then it shows the attempts and time left', async () => {
      expect(await screen.findByText('9m')).toBeInTheDocument();
      expect(screen.getByText('2')).toBeInTheDocument();
    });

    it('Then the summary counts it as recovering', async () => {
      expect(
        await screen.findByText('1 of 1 devices recovering'),
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
      vi.stubGlobal('fetch', fetch);
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

    it('Then the header shows it is reconnecting', async () => {
      expect(await screen.findByText('Reconnecting')).toBeInTheDocument();
    });
  });
});
