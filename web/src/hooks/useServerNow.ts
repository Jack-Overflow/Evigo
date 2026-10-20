import { useSyncExternalStore } from 'react';

let serverOffsetMilliseconds = 0;
let currentServerTime = Date.now();
let timer: ReturnType<typeof setInterval> | undefined;
const listeners = new Set<() => void>();

function readClock() {
  currentServerTime = Date.now() + serverOffsetMilliseconds;
}

function notify() {
  listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void) {
  listeners.add(listener);

  if (!timer) {
    readClock();
    timer = setInterval(() => {
      readClock();
      notify();
    }, 1000);
  }

  return () => {
    listeners.delete(listener);

    if (listeners.size === 0) {
      clearInterval(timer);
      timer = undefined;
    }
  };
}

export function alignWithServerTime(serverTime: string) {
  const parsed = Date.parse(serverTime);

  if (!Number.isFinite(parsed)) {
    return;
  }

  serverOffsetMilliseconds = parsed - Date.now();
  readClock();
  notify();
}

export function useServerNow() {
  return useSyncExternalStore(subscribe, () => currentServerTime);
}
