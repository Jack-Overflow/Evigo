import { useQuery } from '@tanstack/react-query';
import { getStatus } from '../api';
import { useElapsedNow } from './useNow';

const pollMilliseconds = 5000;
const staleMilliseconds = 15000;

export function useStatus() {
  const now = useElapsedNow();
  const query = useQuery({
    queryKey: ['status'],
    queryFn: async ({ signal }) => ({
      status: await getStatus(signal),
      receivedAt: performance.now(),
    }),
    refetchInterval: pollMilliseconds,
  });
  const stale =
    query.isError ||
    !query.data ||
    now - query.data.receivedAt > staleMilliseconds;

  return {
    status: query.data?.status,
    unreachable: query.isError,
    stale,
  };
}
