import { act, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  answerAtTheCurrentTime,
  answerWith,
  nas,
  recoveryOf,
  renderApp,
  statusWith,
} from './testing';

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
      answerWith(
        statusWith({
          server_time: '2026-01-01T12:00:00Z',
          power: {
            block: 'stabilising',
            wake_allowed_at: '2026-01-01T12:00:20Z',
          },
        }),
      );
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
      answerWith(
        statusWith({ power: { block: 'battery_low', battery_percent: 10 } }),
      );
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

  describe('When a device is being woken', () => {
    beforeEach(() => {
      answerWith(
        statusWith({
          devices: [
            nas(
              'offline',
              recoveryOf({
                attempts: 2,
                last_attempt_at: new Date().toISOString(),
                gives_up_at: new Date(Date.now() + 540_000).toISOString(),
              }),
            ),
          ],
        }),
      );
      renderApp();
    });

    it('Then it shows the wake attempts', async () => {
      expect(
        (await screen.findByText('Wake attempts')).nextSibling,
      ).toHaveTextContent('2');
    });

    it('Then it shows the time left to recover', async () => {
      expect(
        (await screen.findByText('Time left to recover')).nextSibling,
      ).toHaveTextContent('9m');
    });
  });

  describe('When a device that stayed on is watched during an outage', () => {
    beforeEach(() => {
      answerWith(
        statusWith({
          power: { block: 'on_battery' },
          devices: [nas('online', recoveryOf({ state: 'watching' }))],
        }),
      );
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
      answerWith(
        statusWith({
          power: { block: 'on_battery' },
          devices: [nas('offline', recoveryOf())],
        }),
      );
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

  describe('When time passes while Evigo is spending the time left', () => {
    beforeEach(async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      vi.setSystemTime(new Date('2026-01-01T12:00:00Z'));
      answerAtTheCurrentTime(
        statusWith({
          devices: [
            nas(
              'offline',
              recoveryOf({ attempts: 1, gives_up_at: '2026-01-01T12:09:00Z' }),
            ),
          ],
        }),
      );
      renderApp();
      await screen.findByText('Time left to recover');
      await act(() => vi.advanceTimersByTimeAsync(30_000));
    });

    it('Then the time left counts down', () => {
      expect(
        screen.getByText('Time left to recover').nextSibling,
      ).toHaveTextContent('8m 30s');
    });
  });

  describe('When Evigo is not spending the time left after a wake attempt', () => {
    beforeEach(() => {
      answerWith(
        statusWith({
          power: { block: 'on_battery' },
          devices: [nas('offline', recoveryOf({ attempts: 1 }))],
        }),
      );
      renderApp();
    });

    it('Then it keeps showing the wake attempts', async () => {
      expect(
        (await screen.findByText('Wake attempts')).nextSibling,
      ).toHaveTextContent('1');
    });

    it('Then it hides the time left', async () => {
      await screen.findByText('Wake attempts');

      expect(
        screen.queryByText('Time left to recover'),
      ).not.toBeInTheDocument();
    });
  });

  describe('When the UPS is settling onto mains power', () => {
    beforeEach(() => {
      answerWith(statusWith({ power: { block: 'settling' } }));
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
      answerWith(statusWith({ power: { block: 'unsafe_status' } }));
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

  describe("When the browser's clock is behind Evigo's", () => {
    beforeEach(() => {
      vi.useFakeTimers({ toFake: ['Date'] });
      vi.setSystemTime(new Date('2026-01-01T11:55:00Z'));
      answerWith(
        statusWith({
          server_time: '2026-01-01T12:00:00Z',
          checked_at: '2026-01-01T12:00:00Z',
          power: {
            block: 'stabilising',
            wake_allowed_at: '2026-01-01T12:00:20Z',
          },
        }),
      );
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
      answerWith(
        statusWith({
          power: {
            block: 'stabilising',
            wake_allowed_at: new Date(Date.now() - 2000).toISOString(),
          },
        }),
      );
      renderApp();
    });

    it('Then it says waking is allowed shortly', async () => {
      expect(
        await screen.findByText('Waking is allowed shortly'),
      ).toBeInTheDocument();
    });
  });
});
