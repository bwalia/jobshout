import { useQuery } from "@tanstack/react-query";
import { getHealth, type HealthInfo } from "@/lib/api/health";

export function useHealth() {
  return useQuery<HealthInfo>({
    queryKey: ["health"],
    queryFn: getHealth,
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
    retry: 1,
  });
}
