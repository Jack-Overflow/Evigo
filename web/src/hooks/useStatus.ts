import { useQuery } from '@tanstack/react-query';
import { getStatus } from '../api';
import { alignWithServerTime } from './useServerNow';

const pollMilliseconds = 5000;
const requestTimeoutMilliseconds = 10000;

export function useStatus() {
  const query = useQuery({
    queryKey: ['status'],
    queryFn: async ({ signal }) => {
      const status = await getStatus(
        AbortSignal.any([
          signal,
          AbortSignal.timeout(requestTimeoutMilliseconds),
        ]),
      );
      alignWithServerTime(status.server_time);

      return status;
    },
    refetchInterval: pollMilliseconds,
  });
  const status = query.data;

  return {
    status,
    unreachable: query.isError,
    stale: query.isError || !status || status.monitor !== 'running',
  };
}
