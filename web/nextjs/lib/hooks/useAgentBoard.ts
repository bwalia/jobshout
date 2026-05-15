import { useQuery } from "@tanstack/react-query";
import { getAgentBoard, type AgentBoardEntry } from "@/lib/api/agent-board";

export const agentBoardKeys = {
  all: ["agent-board"] as const,
};

/**
 * Live agent board. Polls every 5 seconds while the tab is focused so
 * "executing"/"reviewing" agents reflect the current state without making
 * the user click refresh.
 */
export function useAgentBoard() {
  return useQuery<AgentBoardEntry[]>({
    queryKey: agentBoardKeys.all,
    queryFn: getAgentBoard,
    refetchInterval: 5_000,
    refetchIntervalInBackground: false,
  });
}
