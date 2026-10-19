import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { getHistory } from '../api';

const pageSize = 15;
const pollMilliseconds = 5000;

export function useHistory(before: number | undefined) {
  const query = useQuery({
    queryKey: ['history', before],
    queryFn: ({ signal }) => getHistory(pageSize, before, signal),
    refetchInterval: before ? false : pollMilliseconds,
    placeholderData: keepPreviousData,
  });

  return { page: query.data, unavailable: query.isError };
}
