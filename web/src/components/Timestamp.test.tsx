import { render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Timestamp } from './Timestamp';

const now = new Date('2026-01-01T12:00:00Z');

function secondsBefore(seconds: number) {
  return new Date(now.getTime() - seconds * 1000).toISOString();
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(now);
});

afterEach(() => vi.useRealTimers());

describe('Given a timestamp to show', () => {
  describe('When it is under five seconds old', () => {
    beforeEach(() => render(<Timestamp value={secondsBefore(3)} />));

    it('Then it says just now', () => {
      expect(screen.getByText('just now')).toBeInTheDocument();
    });
  });

  describe('When it is 150 seconds old', () => {
    beforeEach(() => render(<Timestamp value={secondsBefore(150)} />));

    it('Then it says how long ago in minutes and seconds', () => {
      expect(screen.getByText('2m 30s ago')).toBeInTheDocument();
    });
  });

  describe('When it is a day and a few hours old', () => {
    beforeEach(() => render(<Timestamp value={secondsBefore(97200)} />));

    it('Then it says how long ago in days and hours', () => {
      expect(screen.getByText('1d 3h ago')).toBeInTheDocument();
    });
  });

  describe('When it is slightly in the future', () => {
    beforeEach(() => render(<Timestamp value={secondsBefore(-2)} />));

    it('Then it says just now rather than a negative time', () => {
      expect(screen.getByText('just now')).toBeInTheDocument();
    });
  });

  describe('When it is more than a minute in the future', () => {
    beforeEach(() => render(<Timestamp value={secondsBefore(-3600)} />));

    it('Then it shows the full date instead of an age', () => {
      expect(screen.getByText(/^01 Jan 2026/)).toBeInTheDocument();
    });
  });

  describe('When there is no timestamp', () => {
    beforeEach(() => render(<Timestamp value={null} empty="No reading yet" />));

    it('Then it shows the empty text', () => {
      expect(screen.getByText('No reading yet')).toBeInTheDocument();
    });
  });

  describe('When it has a full date', () => {
    beforeEach(() => render(<Timestamp value={secondsBefore(150)} />));

    it('Then the full date is available to screen readers', () => {
      expect(
        screen.getByLabelText(/^2m 30s ago, 01 Jan 2026/),
      ).toBeInTheDocument();
    });
  });
});
