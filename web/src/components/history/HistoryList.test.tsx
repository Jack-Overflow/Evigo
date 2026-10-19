import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Device, HistoryEvent } from '../../api';
import { HistoryList } from './HistoryList';

const devices: Device[] = [
  {
    id: 'nas',
    name: 'NAS',
    host: '192.0.2.10',
    mac: '02:00:00:00:00:10',
    reachability: 'online',
    recovery: null,
  },
];

function minutesAgo(minutes: number) {
  return new Date(Date.now() - minutes * 60_000).toISOString();
}

function answerWith(events: HistoryEvent[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => Response.json({ events, next_before: null })),
  );
}

function answerWithTwoPages() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('before=20')) {
        return Response.json({
          events: [
            {
              id: 19,
              at: minutesAgo(60),
              kind: 'started',
              details: { version: '0.2.0' },
            },
          ],
          next_before: null,
        });
      }

      return Response.json({
        events: [
          {
            id: 20,
            at: minutesAgo(1),
            kind: 'device_recovered',
            device_id: 'nas',
            details: { attempt: 1 },
          },
        ],
        next_before: 20,
      });
    }),
  );
}

function renderHistory() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  render(
    <QueryClientProvider client={client}>
      <HistoryList devices={devices} />
    </QueryClientProvider>,
  );
}

async function rows() {
  return within(await screen.findByRole('list')).getAllByRole('listitem');
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("Given a request for Evigo's history", () => {
  describe('When events have been recorded', () => {
    beforeEach(() => {
      answerWith([
        {
          id: 4,
          at: minutesAgo(2),
          kind: 'wake_sent',
          device_id: 'nas',
          details: { source: 'automatic', attempt: 2 },
        },
        {
          id: 3,
          at: minutesAgo(5),
          kind: 'reachability',
          device_id: 'nas',
          details: { reachability: 'offline' },
        },
        {
          id: 2,
          at: minutesAgo(9),
          kind: 'ups_status',
          details: { status: 'OB DISCHRG', battery_percent: 80 },
        },
        {
          id: 1,
          at: minutesAgo(30),
          kind: 'device_recovered',
          device_id: 'removed-device',
          details: { attempt: 1 },
        },
      ]);
      renderHistory();
    });

    it('Then each is described in the order Evigo returned them', async () => {
      expect((await rows()).map((row) => row.textContent)).toEqual([
        'Wake sent to NAS (attempt 2)2m ago',
        'NAS is not responding5m ago',
        'UPS: On battery · Discharging · 80% charge9m ago',
        'removed-device recovered30m ago',
      ]);
    });
  });

  describe('When the UPS could not be read', () => {
    beforeEach(() => {
      answerWith([
        {
          id: 1,
          at: minutesAgo(1),
          kind: 'ups_status',
          details: { status: 'unavailable' },
        },
      ]);
      renderHistory();
    });

    it('Then the event says there was no reading', async () => {
      expect((await rows())[0]).toHaveTextContent('UPS: no reading');
    });
  });

  describe('When older events are asked for', () => {
    beforeEach(async () => {
      answerWithTwoPages();
      renderHistory();
      await userEvent.click(
        await screen.findByRole('button', { name: 'Older' }),
      );
    });

    it('Then it shows the next page', async () => {
      expect(
        await screen.findByText('Evigo 0.2.0 started'),
      ).toBeInTheDocument();
    });

    it('Then there is nothing older to ask for', async () => {
      await screen.findByText('Evigo 0.2.0 started');

      expect(screen.getByRole('button', { name: 'Older' })).toBeDisabled();
    });
  });

  describe('When newer events are asked for after older ones', () => {
    beforeEach(async () => {
      answerWithTwoPages();
      renderHistory();
      await userEvent.click(
        await screen.findByRole('button', { name: 'Older' }),
      );
      await screen.findByText('Evigo 0.2.0 started');
      await userEvent.click(screen.getByRole('button', { name: 'Newer' }));
    });

    it('Then it shows the latest page again', async () => {
      expect(await screen.findByText('NAS recovered')).toBeInTheDocument();
    });

    it('Then there is nothing newer to ask for', async () => {
      await screen.findByText('NAS recovered');

      expect(screen.getByRole('button', { name: 'Newer' })).toBeDisabled();
    });
  });

  describe('When nothing has been recorded', () => {
    beforeEach(() => {
      answerWith([]);
      renderHistory();
    });

    it('Then it says nothing has been recorded yet', async () => {
      expect(
        await screen.findByText('Nothing recorded yet'),
      ).toBeInTheDocument();
    });
  });

  describe('When history cannot be read', () => {
    beforeEach(() => {
      vi.stubGlobal(
        'fetch',
        vi.fn(async () =>
          Response.json({ error: 'unavailable' }, { status: 503 }),
        ),
      );
      renderHistory();
    });

    it('Then it says history is unavailable', async () => {
      expect(
        await screen.findByText('History is unavailable'),
      ).toBeInTheDocument();
    });
  });
});
