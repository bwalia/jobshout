import {
  useQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { toast } from "sonner";
import {
  getSessions,
  getSession,
  createSession,
  updateSession,
  deleteSession,
  copySessionContext,
  createSessionSnapshot,
  getSessionSnapshots,
  restoreSessionSnapshot,
} from "@/lib/api/sessions";
import type { PaginationParams } from "@/lib/types/common";
import type {
  CreateSessionRequest,
  UpdateSessionRequest,
  CopyContextRequest,
  CreateSnapshotRequest,
} from "@/lib/types/session";

export const sessionKeys = {
  all: ["sessions"] as const,
  lists: () => [...sessionKeys.all, "list"] as const,
  list: (p: PaginationParams) => [...sessionKeys.lists(), p] as const,
  detail: (id: string) => [...sessionKeys.all, "detail", id] as const,
  snapshots: (id: string) => [...sessionKeys.all, "snapshots", id] as const,
};

export function useSessions(params: PaginationParams = {}) {
  return useQuery({
    queryKey: sessionKeys.list(params),
    queryFn: () => getSessions(params),
  });
}

export function useSession(id: string) {
  return useQuery({
    queryKey: sessionKeys.detail(id),
    queryFn: () => getSession(id),
    enabled: Boolean(id),
  });
}

export function useCreateSession() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createSession,
    onSuccess: (s) => {
      qc.invalidateQueries({ queryKey: sessionKeys.lists() });
      toast.success(`Session "${s.name}" created.`);
    },
    onError: (e: Error) => toast.error(`Failed: ${e.message}`),
  });
}

export function useUpdateSession() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: UpdateSessionRequest }) =>
      updateSession(id, payload),
    onSuccess: (s) => {
      qc.invalidateQueries({ queryKey: sessionKeys.lists() });
      qc.invalidateQueries({ queryKey: sessionKeys.detail(s.id) });
      toast.success("Session updated.");
    },
    onError: (e: Error) => toast.error(`Failed: ${e.message}`),
  });
}

export function useDeleteSession() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteSession,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: sessionKeys.lists() });
      toast.success("Session deleted.");
    },
    onError: (e: Error) => toast.error(`Failed: ${e.message}`),
  });
}

export function useCopySessionContext() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ sessionId, payload }: { sessionId: string; payload: CopyContextRequest }) =>
      copySessionContext(sessionId, payload),
    onSuccess: (s) => {
      qc.invalidateQueries({ queryKey: sessionKeys.detail(s.id) });
      toast.success("Context copied successfully.");
    },
    onError: (e: Error) => toast.error(`Failed: ${e.message}`),
  });
}

export function useSessionSnapshots(sessionId: string) {
  return useQuery({
    queryKey: sessionKeys.snapshots(sessionId),
    queryFn: () => getSessionSnapshots(sessionId),
    enabled: Boolean(sessionId),
  });
}

export function useCreateSnapshot() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ sessionId, payload }: { sessionId: string; payload: CreateSnapshotRequest }) =>
      createSessionSnapshot(sessionId, payload),
    onSuccess: (snap) => {
      qc.invalidateQueries({ queryKey: sessionKeys.snapshots(snap.session_id) });
      toast.success("Snapshot saved.");
    },
    onError: (e: Error) => toast.error(`Failed: ${e.message}`),
  });
}

export function useRestoreSnapshot() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ sessionId, snapshotId }: { sessionId: string; snapshotId: string }) =>
      restoreSessionSnapshot(sessionId, snapshotId),
    onSuccess: (s) => {
      qc.invalidateQueries({ queryKey: sessionKeys.detail(s.id) });
      toast.success("Snapshot restored.");
    },
    onError: (e: Error) => toast.error(`Failed: ${e.message}`),
  });
}
