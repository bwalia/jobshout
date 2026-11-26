"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  approveFacebookPost,
  deleteFacebookPost,
  disconnectFacebook,
  editPublishedFacebookPost,
  generateFacebookImage,
  getFacebookAnalytics,
  getFacebookStatus,
  listFacebookComments,
  listFacebookPosts,
  publishFacebookPost,
  redraftFacebookPost,
  refreshFacebookPage,
  setFacebookAutomation,
  replyToFacebookComment,
  saveFacebookProfile,
  startFacebookConnect,
  unscheduleFacebookPost,
  updateFacebookPageInfo,
  updateFacebookPost,
} from "@/lib/api/facebook";
import { apiErrorMessage } from "@/lib/api/client";
import { isFacebookPostActive, type FacebookPost } from "@/lib/types/facebook";

export const facebookKeys = {
  all: ["facebook"] as const,
  status: () => [...facebookKeys.all, "status"] as const,
  posts: (pageId?: string) => [...facebookKeys.all, "posts", pageId ?? "all"] as const,
  analytics: (pageId: string) => [...facebookKeys.all, "analytics", pageId] as const,
  comments: (postId: string) => [...facebookKeys.all, "comments", postId] as const,
};

export function useFacebookStatus() {
  return useQuery({ queryKey: facebookKeys.status(), queryFn: getFacebookStatus });
}

/** Posts, polled while any is drafting or publishing. */
export function useFacebookPosts(pageId?: string) {
  return useQuery({
    queryKey: facebookKeys.posts(pageId),
    queryFn: () => listFacebookPosts(pageId),
    refetchInterval: (query) =>
      query.state.data?.some((p: FacebookPost) => isFacebookPostActive(p.status)) ? 3000 : false,
  });
}

export function useFacebookAnalytics(pageId: string | undefined, enabled: boolean) {
  return useQuery({
    queryKey: facebookKeys.analytics(pageId ?? ""),
    queryFn: () => getFacebookAnalytics(pageId as string),
    enabled: Boolean(pageId) && enabled,
    staleTime: 60_000,
  });
}

export function useFacebookComments(postId: string, enabled: boolean) {
  return useQuery({
    queryKey: facebookKeys.comments(postId),
    queryFn: () => listFacebookComments(postId),
    enabled,
  });
}

/** A mutation that refreshes posts (and status) afterwards and toasts the outcome. */
function usePostAction<V = void>(
  fn: (v: V) => Promise<unknown>,
  ok: string,
  fail: string,
  alsoStatus = false
) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [...facebookKeys.all, "posts"] });
      if (alsoStatus) qc.invalidateQueries({ queryKey: facebookKeys.all });
      if (ok) toast.success(ok);
    },
    onError: (e) => {
      // A rejected token or a half-finished publish changes more than one row.
      qc.invalidateQueries({ queryKey: facebookKeys.all });
      toast.error(apiErrorMessage(e, fail));
    },
  });
}

export function useConnectFacebook() {
  return useMutation({
    mutationFn: startFacebookConnect,
    onSuccess: (url) => {
      window.location.href = url;
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Could not start the Facebook connection")),
  });
}

export const useDisconnectFacebook = () =>
  usePostAction<void>(disconnectFacebook, "Facebook disconnected", "Could not disconnect Facebook", true);

export const useSaveFacebookProfile = () =>
  usePostAction(
    (v: { pageId: string; brief: string; voice: string; timezone: string }) =>
      saveFacebookProfile(v.pageId, { brief: v.brief, voice: v.voice, timezone: v.timezone }),
    "Page profile saved",
    "Could not save the profile",
    true
  );

export const useSetFacebookAutomation = () =>
  usePostAction(
    (v: { pageId: string; paused: boolean }) => setFacebookAutomation(v.pageId, v.paused),
    "Automation updated",
    "Could not update automation",
    true
  );

export const useRefreshFacebookPage = () =>
  usePostAction(refreshFacebookPage, "Page details refreshed", "Could not refresh the Page", true);

export const useUpdateFacebookPageInfo = () =>
  usePostAction(
    (v: { pageId: string; about?: string; description?: string; website?: string }) =>
      updateFacebookPageInfo(v.pageId, { about: v.about, description: v.description, website: v.website }),
    "Page updated on Facebook",
    "Could not update the Page",
    true
  );

export const useUpdateFacebookPost = () =>
  usePostAction(
    (v: { id: string; message: string; scheduled_for?: string; clear_schedule?: boolean }) =>
      updateFacebookPost(v.id, {
        message: v.message,
        scheduled_for: v.scheduled_for,
        clear_schedule: v.clear_schedule,
      }),
    "Draft saved",
    "Could not save the draft"
  );

export const useEditPublishedFacebookPost = () =>
  usePostAction(
    (v: { id: string; message: string }) => editPublishedFacebookPost(v.id, v.message),
    "Post updated on Facebook",
    "Could not update the post"
  );

export const useApproveFacebookPost = () =>
  usePostAction(
    (v: { id: string; scheduledFor?: string }) => approveFacebookPost(v.id, v.scheduledFor),
    "Approved",
    "Could not approve the post"
  );

export const usePublishFacebookPost = () =>
  usePostAction(publishFacebookPost, "Published on Facebook", "Could not publish to Facebook");

export const useUnscheduleFacebookPost = () =>
  usePostAction(unscheduleFacebookPost, "Moved back to draft", "Could not unschedule the post");

export const useRedraftFacebookPost = () =>
  usePostAction(redraftFacebookPost, "Redrafting", "Could not redraft");

export const useGenerateFacebookImage = () =>
  usePostAction(generateFacebookImage, "Creative generated", "Could not generate the creative");

export const useDeleteFacebookPost = () =>
  usePostAction(deleteFacebookPost, "Post deleted", "Could not delete the post");

export function useReplyToFacebookComment(postId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { commentId: string; message: string }) =>
      replyToFacebookComment(postId, v.commentId, v.message),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: facebookKeys.comments(postId) });
      toast.success("Reply posted");
    },
    onError: (e) => toast.error(apiErrorMessage(e, "Could not post the reply")),
  });
}
