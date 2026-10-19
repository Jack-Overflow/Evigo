import { beforeEach, describe, expect, it } from 'vitest';
import { durationText, reachabilityText, upsStatusText } from './format';

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
