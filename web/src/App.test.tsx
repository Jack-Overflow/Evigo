import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from './App';
import type { Status } from './api';

const status: Status = {
  version: '0.2.0',
  checked_at: '2026-01-01T12:00:00Z',
  power: {
    ups: 'main-ups',
    available: true,
    status: ['OL'],
    battery_percent: 100,
    read_at: '2026-01-01T12:00:00Z',
    on_battery: false,
    block: null,
    stable_since: '2026-01-01T11:00:00Z',
    wake_allowed_at: '2026-01-01T11:00:30Z',
  },
  storage_available: true,
  settings: {
    min_battery_percent: 25,
    stable_power_seconds: 30,
    shutdown_grace_seconds: 120,
    retry_seconds: 30,
    device_timeout_seconds: 600,
  },
  devices: [],
};

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

afterEach(() => vi.unstubAllGlobals());

describe("Given a request for Evigo's status", () => {
  describe('When Evigo is available', () => {
    beforeEach(() => {
      vi.stubGlobal(
        'fetch',
        vi.fn(async () => Response.json(status)),
      );
      renderApp();
    });

    it('Then it shows the version', async () => {
      expect(await screen.findByText('v0.2.0')).toBeInTheDocument();
    });

    it('Then it shows what is being watched', async () => {
      expect(
        await screen.findByText('Watching main-ups and 0 machines.'),
      ).toBeInTheDocument();
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
});
