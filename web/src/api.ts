export type Reachability = 'unknown' | 'offline' | 'online';

export type PowerBlock =
  'no_reading' | 'on_battery' | 'battery_low' | 'unsafe_status' | 'stabilising';

export interface Power {
  ups: string;
  available: boolean;
  status: string[];
  battery_percent: number | null;
  read_at: string | null;
  on_battery: boolean;
  block: PowerBlock | null;
  stable_since: string | null;
  wake_allowed_at: string | null;
}

export interface Settings {
  min_battery_percent: number;
  stable_power_seconds: number;
  shutdown_grace_seconds: number;
  retry_seconds: number;
  device_timeout_seconds: number;
}

export interface DeviceRecovery {
  attempts: number;
  last_attempt_at: string | null;
  time_left_seconds: number;
  grace_left_seconds: number;
  wake_sent: boolean;
}

export interface Device {
  id: string;
  name: string;
  host: string;
  mac: string;
  reachability: Reachability;
  recovery: DeviceRecovery | null;
}

export type MonitorState = 'running' | 'stalled' | 'starting';

export interface Status {
  version: string;
  server_time: string;
  monitor: MonitorState;
  checked_at: string | null;
  power: Power;
  storage_available: boolean;
  settings: Settings;
  devices: Device[];
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(code);
  }
}

async function request<T>(path: string, signal?: AbortSignal): Promise<T> {
  let response: Response;

  try {
    response = await fetch(`/api${path}`, { signal });
  } catch (error) {
    if (signal?.aborted) {
      throw error;
    }

    throw new ApiError(0, 'unavailable');
  }

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as {
      error?: string;
    };

    throw new ApiError(response.status, body.error ?? 'unavailable');
  }

  return (await response.json()) as T;
}

export function getStatus(signal?: AbortSignal) {
  return request<Status>('/status', signal);
}
