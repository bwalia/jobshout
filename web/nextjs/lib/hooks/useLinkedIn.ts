"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  disconnectLinkedIn,
  getLinkedInStatus,
  listLinkedInPosts,
  publishLinkedInPost,
  redraftLinkedInPost,
  startLinkedInConnect,
  updateLinkedInPost,
} from "@/lib/api/linkedin";
import { apiErrorMessage } from "@/lib/api/client";
import { isLinkedInPostActive, type LinkedInPost } from "@/lib/types/linkedin";

export const linkedInKeys = {
  all: ["linkedin"] as const,
  status: () => [...linkedInKeys.all, "status"] as const,
  posts: () => [...linkedInKeys.all, "posts"] as const,
};

export function useLinkedInStatus() {
  return useQuery({ queryKey: linkedInKeys.status(), queryFn: getLinkedInStatus });
}

/** Drafts and posts, polled while any is drafting or posting. */
export function useLinkedInPosts() {
  return useQuery({
    queryKey: linkedInKeys.posts(),
    queryFn: listLinkedInPosts,
    refetchInterval: (query) =>
      query.state.data?.some((p: LinkedInPost) => isLinkedInPostActive(p.status)) ? 3000 : false,
  });
}

export function useConnectLinkedIn() {
  return useMutation({
    mutationFn: startLinkedInConnect,
    onSuccess: (url) => {
      window.location.href = url;
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Could not start the LinkedIn connection")),
  });
}

export function useDisconnectLinkedIn() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: disconnectLinkedIn,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: linkedInKeys.status() });
      toast.success("LinkedIn disconnected");
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Could not disconnect LinkedIn")),
  });
}

export function useUpdateLinkedInPost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, commentary }: { id: string; commentary: string }) =>
      updateLinkedInPost(id, commentary),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: linkedInKeys.posts() });
      toast.success("Draft saved");
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Could not save the draft")),
  });
}

export function useRedraftLinkedInPost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => redraftLinkedInPost(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: linkedInKeys.posts() });
      toast.success("Redrafting");
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Could not redraft")),
  });
}

export function usePublishLinkedInPost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => publishLinkedInPost(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: linkedInKeys.posts() });
      toast.success("Posted on LinkedIn");
    },
    onError: (e) => {
      qc.invalidateQueries({ queryKey: linkedInKeys.posts() });
      qc.invalidateQueries({ queryKey: linkedInKeys.status() });
      toast.error(apiErrorMessage(e, "Could not post to LinkedIn"));
    },
  });
}
