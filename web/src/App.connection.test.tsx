import { act, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  answerWith,
  renderApp,
  statusWith,
  stubStatusFetch,
  timeOutWithFakeTimers,
} from './testing';

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("Given a request for Evigo's status", () => {
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
