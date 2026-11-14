import { API_BASE, failure, headers, type Viewer } from "@/lib/insights";

export type AppNotification = {
  id: string;
  kind: string;
  title: string;
  body: string;
  href: string;
  read_at: string | null;
  created_at: string;
};

export type NotificationPreference = { kind: string; label: string; in_app: boolean };

export async function listNotifications(
  viewer: Viewer,
  limit = 50,
): Promise<{ data: AppNotification[]; unread: number }> {
  const res = await fetch(`${API_BASE}/api/v1/notifications?limit=${limit}`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to load notifications");
  return (await res.json()) as { data: AppNotification[]; unread: number };
}

/** Unread count for the header; 0 on any failure so the header never breaks. */
export async function unreadNotifications(viewer: Viewer): Promise<number> {
  try {
    const res = await fetch(`${API_BASE}/api/v1/notifications?limit=1&unread=true`, {
      headers: headers(viewer),
      cache: "no-store",
    });
    if (!res.ok) return 0;
    return ((await res.json()) as { unread: number }).unread;
  } catch {
    return 0;
  }
}

/** Mark some (or, with no ids, all) notifications read. */
export async function markNotificationsRead(viewer: Viewer, ids: string[] = []): Promise<void> {
  const res = await fetch(`${API_BASE}/api/v1/notifications/read`, {
    method: "POST",
    headers: headers(viewer),
    body: JSON.stringify({ ids }),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Could not mark notifications read");
}

export async function getNotificationPreferences(viewer: Viewer): Promise<NotificationPreference[]> {
  const res = await fetch(`${API_BASE}/api/v1/notifications/preferences`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Failed to load notification settings");
  return (await res.json()) as NotificationPreference[];
}

export async function setNotificationPreference(
  viewer: Viewer,
  kind: string,
  inApp: boolean,
): Promise<void> {
  const res = await fetch(`${API_BASE}/api/v1/notifications/preferences`, {
    method: "PUT",
    headers: headers(viewer),
    body: JSON.stringify({ kind, in_app: inApp }),
    cache: "no-store",
  });
  if (!res.ok) throw await failure(res, "Could not save the setting");
}
