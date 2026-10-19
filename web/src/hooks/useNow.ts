import { useSyncExternalStore } from 'react';

let wallClock = Date.now();
let elapsedClock = performance.now();
let timer: ReturnType<typeof setInterval> | undefined;
const listeners = new Set<() => void>();

function tick() {
  wallClock = Date.now();
  elapsedClock = performance.now();
}

function subscribe(listener: () => void) {
  listeners.add(listener);

  if (!timer) {
    tick();
    timer = setInterval(() => {
      tick();
      listeners.forEach((notify) => notify());
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

export function useNow() {
  return useSyncExternalStore(subscribe, () => wallClock);
}

// Elapsed time ignores server timestamps and wall-clock jumps.
export function useElapsedNow() {
  return useSyncExternalStore(subscribe, () => elapsedClock);
}
