import { beforeEach, describe, expect, it } from 'vitest';
import type { Power, Settings, Status } from '../api';
import {
  ageText,
  blockText,
  connectionNotice,
  durationText,
  gaugeCaption,
  gaugeValueText,
  powerDetail,
  powerHeadline,
  reachabilityText,
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

function statusWith(monitor: Status['monitor']): Status {
  return {
    version: '0.2.0',
    server_time: '2026-01-01T12:00:00Z',
    checked_at: '2026-01-01T12:00:00Z',
    monitor,
    power,
    storage_available: true,
    settings,
    devices: [],
  };
}

describe('Given a count of recovering devices', () => {
  let text: string;

  describe('When some are recovering', () => {
    beforeEach(() => {
      text = recoverySummary(1, 2);
    });

    it('Then it says how many of the total', () => {
      expect(text).toBe('1 of 2 devices recovering');
    });
  });

  describe('When none are recovering', () => {
    beforeEach(() => {
      text = recoverySummary(0, 2);
    });

    it('Then it says there is nothing to recover', () => {
      expect(text).toBe('Nothing to recover');
    });
  });
});

describe('Given a duration in seconds', () => {
  let text: string;

  describe('When it is under a minute', () => {
    beforeEach(() => {
      text = durationText(45);
    });

    it('Then it is shown in seconds', () => {
      expect(text).toBe('45s');
    });
  });

  describe('When it is minutes with leftover seconds', () => {
    beforeEach(() => {
      text = durationText(150);
    });

    it('Then it includes the leftover seconds', () => {
      expect(text).toBe('2m 30s');
    });
  });

  describe('When it is a whole number of hours', () => {
    beforeEach(() => {
      text = durationText(7200);
    });

    it('Then the zero part is dropped', () => {
      expect(text).toBe('2h');
    });
  });

  describe('When it is hours with leftover minutes', () => {
    beforeEach(() => {
      text = durationText(3900);
    });

    it('Then it includes the leftover minutes', () => {
      expect(text).toBe('1h 5m');
    });
  });

  describe('When it is days with leftover hours', () => {
    beforeEach(() => {
      text = durationText(183600);
    });

    it('Then it includes the leftover hours', () => {
      expect(text).toBe('2d 3h');
    });
  });
});

describe('Given the age of a timestamp in seconds', () => {
  let text: string | null;

  describe('When it is under five seconds', () => {
    beforeEach(() => {
      text = ageText(3);
    });

    it('Then it says just now', () => {
      expect(text).toBe('just now');
    });
  });

  describe('When it is slightly in the future', () => {
    beforeEach(() => {
      text = ageText(-30);
    });

    it('Then it still says just now', () => {
      expect(text).toBe('just now');
    });
  });

  describe('When it is more than a minute in the future', () => {
    beforeEach(() => {
      text = ageText(-120);
    });

    it('Then it gives no age so the full date is shown instead', () => {
      expect(text).toBeNull();
    });
  });

  describe('When it is in the past', () => {
    beforeEach(() => {
      text = ageText(150);
    });

    it('Then it says how long ago', () => {
      expect(text).toBe('2m 30s ago');
    });
  });
});

describe('Given the power state', () => {
  describe('When nothing blocks waking', () => {
    let headline: ReturnType<typeof powerHeadline>;
    let detail: string;

    beforeEach(() => {
      headline = powerHeadline(power);
      detail = powerDetail(power, settings, null, false);
    });

    it('Then the headline says power is OK', () => {
      expect(headline).toEqual({ title: 'Power is OK', tone: 'good' });
    });

    it('Then the detail says mains power is stable', () => {
      expect(detail).toBe('Mains power is on and stable');
    });
  });

  describe('When the battery is below the minimum', () => {
    let text: string;

    beforeEach(() => {
      text = blockText('battery_low', settings, null);
    });

    it('Then the reason names the minimum', () => {
      expect(text).toBe(
        'Waiting for the battery to reach 25% before waking anything',
      );
    });
  });

  describe('When power is stabilising with part of a second left', () => {
    let text: string;

    beforeEach(() => {
      text = blockText('stabilising', settings, 17.2);
    });

    it('Then the countdown rounds up', () => {
      expect(text).toBe('Waking is allowed in 18s');
    });
  });

  describe('When power is stabilising past its wake time', () => {
    let text: string;

    beforeEach(() => {
      text = blockText('stabilising', settings, -3);
    });

    it('Then it says waking is allowed shortly', () => {
      expect(text).toBe('Waking is allowed shortly');
    });
  });

  describe('When there is no fresh UPS reading', () => {
    let title: string;

    beforeEach(() => {
      title = powerHeadline({ ...power, block: 'no_reading' }).title;
    });

    it('Then the headline says power is unknown', () => {
      expect(title).toBe('Power status unknown');
    });
  });

  describe('When the status is outdated', () => {
    let detail: string;

    beforeEach(() => {
      detail = powerDetail(
        { ...power, block: 'stabilising' },
        settings,
        10,
        true,
      );
    });

    it('Then the detail says it is the last known state', () => {
      expect(detail).toBe(
        'Showing the last known state until Evigo answers again',
      );
    });
  });
});

describe('Given a battery reading', () => {
  describe('When it is valid', () => {
    let valueText: string;
    let caption: string;

    beforeEach(() => {
      valueText = gaugeValueText(80.4, 25, true);
      caption = gaugeCaption(25, true);
    });

    it('Then it describes the charge against the minimum', () => {
      expect(valueText).toBe('80% charge; waking needs 25%');
    });

    it('Then the caption names the minimum', () => {
      expect(caption).toBe('Waking needs at least 25%');
    });
  });

  describe('When it is the last known reading', () => {
    let valueText: string;
    let caption: string;

    beforeEach(() => {
      valueText = gaugeValueText(null, 25, false);
      caption = gaugeCaption(25, false);
    });

    it('Then it says the reading is not valid', () => {
      expect(valueText).toBe(
        'Unknown charge; waking needs 25%; last known reading',
      );
    });

    it('Then the caption says it is not fresh', () => {
      expect(caption).toBe('Last known value, not a fresh reading');
    });
  });
});

describe('Given the connection to Evigo', () => {
  let notice: string | null;

  describe('When Evigo answers and its monitor is running', () => {
    beforeEach(() => {
      notice = connectionNotice(false, statusWith('running'));
    });

    it('Then there is no notice', () => {
      expect(notice).toBeNull();
    });
  });

  describe('When Evigo has never answered', () => {
    beforeEach(() => {
      notice = connectionNotice(true, undefined);
    });

    it('Then it says Evigo is not responding', () => {
      expect(notice).toBe('Evigo is not responding.');
    });
  });

  describe('When Evigo answers but its monitor has stalled', () => {
    beforeEach(() => {
      notice = connectionNotice(false, statusWith('stalled'));
    });

    it('Then it says checks have stopped', () => {
      expect(notice).toBe(
        'Evigo has stopped checking the UPS and devices. Showing the last known state.',
      );
    });
  });

  describe('When Evigo answers but has not finished its first check', () => {
    beforeEach(() => {
      notice = connectionNotice(false, statusWith('starting'));
    });

    it('Then it says Evigo is starting', () => {
      expect(notice).toBe(
        'Evigo is starting and has not checked the UPS and devices yet.',
      );
    });
  });
});

describe('Given UPS status flags', () => {
  let text: string;

  describe('When every flag is known', () => {
    beforeEach(() => {
      text = upsStatusText(['OL', 'CHRG']);
    });

    it('Then each flag is named', () => {
      expect(text).toBe('On mains · Charging');
    });
  });

  describe('When there are no flags', () => {
    beforeEach(() => {
      text = upsStatusText([]);
    });

    it('Then the status is unknown', () => {
      expect(text).toBe('Unknown');
    });
  });

  describe('When Evigo did not recognise a flag', () => {
    beforeEach(() => {
      text = upsStatusText(['OL', 'UNKNOWN']);
    });

    it('Then it says so', () => {
      expect(text).toBe('On mains · Unrecognised status');
    });
  });
});

describe("Given a device's reachability", () => {
  let reachability: ReturnType<typeof reachabilityText>;

  describe('When the status is current', () => {
    beforeEach(() => {
      reachability = reachabilityText('offline', false);
    });

    it('Then it says whether the device responds', () => {
      expect(reachability).toEqual({ label: 'Not responding', tone: 'warn' });
    });
  });

  describe('When the status is outdated', () => {
    beforeEach(() => {
      reachability = reachabilityText('online', true);
    });

    it('Then it is marked as outdated', () => {
      expect(reachability).toEqual({ label: 'Outdated', tone: 'neutral' });
    });
  });
});
