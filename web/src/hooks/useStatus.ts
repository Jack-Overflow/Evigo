import { useQuery } from '@tanstack/react-query';
import { getStatus } from '../api';

const pollMilliseconds = 5000;

export function useStatus() {
  const query = useQuery({
    queryKey: ['status'],
    queryFn: ({ signal }) => getStatus(signal),
    refetchInterval: pollMilliseconds,
  });

  return { status: query.data, unreachable: query.isError };
}
