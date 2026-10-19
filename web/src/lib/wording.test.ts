import { describe, expect, it } from 'vitest';
import type { Device, Power, Settings } from '../api';
import {
  blockText,
  deviceStageText,
  durationText,
  powerHeadline,
  recoverySummary,
  upsStatusText,
} from './wording';

const settings: Settings = {
  min_battery_percent: 25,
  stable_power_seconds: 30,
  shutdown_grace_seconds: 120,
  retry_seconds: 30,
  device_timeout_seconds: 600,
};

const power: Power = {
  ups: 'main-ups',
  available: true,
  status: ['OL'],
  battery_percent: 100,
  read_at: null,
  on_battery: false,
  block: null,
  stable_since: null,
  wake_allowed_at: null,
};

function device(overrides: Partial<Device>): Device {
  return {
    id: 'server',
    name: 'Server',
    host: '192.0.2.10',
    mac: '02:00:00:00:00:10',
    reachability: 'online',
    recovery: null,
    ...overrides,
  };
}

const recovering = {
  attempts: 1,
  last_attempt_at: null,
  time_left_seconds: 540,
  grace_left_seconds: 0,
  wake_sent: false,
};

describe('Given a count of recovering devices', () => {
  describe('When some are recovering', () => {
    it('Then it says how many of the total', () => {
      expect(recoverySummary(1, 2)).toBe('1 of 2 devices recovering');
    });
  });

  describe('When none are recovering', () => {
    it('Then it says there is nothing to recover', () => {
      expect(recoverySummary(0, 2)).toBe('Nothing to recover');
    });
  });
});

describe('Given a duration in seconds', () => {
  describe('When it is turned into words', () => {
    it('Then under a minute is shown in seconds', () => {
      expect(durationText(45)).toBe('45s');
    });

    it('Then minutes include leftover seconds', () => {
      expect(durationText(150)).toBe('2m 30s');
    });

    it('Then whole minutes and hours drop the zero part', () => {
      expect([durationText(120), durationText(7200)]).toEqual(['2m', '2h']);
    });

    it('Then hours include leftover minutes', () => {
      expect(durationText(3900)).toBe('1h 5m');
    });

    it('Then days include leftover hours', () => {
      expect(durationText(183600)).toBe('2d 3h');
    });
  });
});

describe('Given the power state', () => {
  describe('When nothing blocks waking', () => {
    it('Then the headline says power is OK', () => {
      expect(powerHeadline(power)).toEqual({
        title: 'Power is OK',
        tone: 'good',
      });
    });
  });

  describe('When the battery is below the minimum', () => {
    it('Then the reason names the minimum', () => {
      expect(blockText('battery_low', settings, null)).toBe(
        'Waiting for the battery to reach 25% before waking anything',
      );
    });
  });

  describe('When power is stabilising with a known wake time', () => {
    it('Then the reason counts down to it', () => {
      expect(blockText('stabilising', settings, 18)).toBe(
        'Waking is allowed in 18s',
      );
    });
  });

  describe('When there is no fresh UPS reading', () => {
    it('Then the headline says power is unknown', () => {
      expect(powerHeadline({ ...power, block: 'no_reading' }).title).toBe(
        'Power status unknown',
      );
    });
  });
});

describe('Given UPS status flags', () => {
  describe('When they are turned into words', () => {
    it('Then each known flag is named', () => {
      expect(upsStatusText(['OL', 'CHRG'])).toBe('On mains · Charging');
    });

    it('Then an empty status is unknown', () => {
      expect(upsStatusText([])).toBe('Unknown');
    });

    it('Then a flag Evigo did not recognise says so', () => {
      expect(upsStatusText(['OL', 'UNKNOWN'])).toBe(
        'On mains · Unrecognised status',
      );
    });
  });
});

describe('Given a device', () => {
  describe('When it is not recovering and is offline', () => {
    it('Then it explains Evigo only wakes devices that were on', () => {
      expect(deviceStageText(device({ reachability: 'offline' }), null)).toBe(
        'Not running. Evigo only wakes devices that were on before an outage.',
      );
    });
  });

  describe('When it is recovering but power is not ready', () => {
    it('Then it waits for power', () => {
      expect(
        deviceStageText(
          device({ reachability: 'offline', recovery: recovering }),
          'stabilising',
        ),
      ).toBe('Recovering. It will be woken once power is ready.');
    });
  });

  describe('When it is recovering and still answering before any wake', () => {
    it('Then it is given time to finish shutting down', () => {
      expect(deviceStageText(device({ recovery: recovering }), null)).toBe(
        'Still answering. Giving it time to finish shutting down.',
      );
    });
  });

  describe('When a wake has been sent', () => {
    it('Then it waits for the device to come back', () => {
      expect(
        deviceStageText(
          device({
            reachability: 'offline',
            recovery: { ...recovering, wake_sent: true },
          }),
          null,
        ),
      ).toBe('Wake sent. Waiting for it to come back online.');
    });
  });

  describe('When it is recovering but cannot be checked', () => {
    it('Then it is not woken yet', () => {
      expect(
        deviceStageText(
          device({ reachability: 'unknown', recovery: recovering }),
          null,
        ),
      ).toBe('Recovering, but Evigo cannot check it, so it is not woken yet');
    });
  });
});
