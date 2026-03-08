"use client";

import { useRef, useState } from "react";
import { toast } from "sonner";
import { apiClient } from "@/lib/api/client";
import { useAuthStore } from "@/lib/store/auth-store";
import type { AuthUser } from "@/lib/auth/auth";

export default function SettingsPage() {
  const user = useAuthStore((s) => s.user);

  // Initialise form fields from the authenticated user; fall back to empty strings
  const [fullName, setFullName] = useState(user?.full_name ?? "");
  const [email] = useState(user?.email ?? "");
  const [orgName, setOrgName] = useState("");

  // Avatar display: prefer live state over the store value so an upload is
  // reflected immediately without a page reload.
  const [avatarUrl, setAvatarUrl] = useState<string | null>(
    user?.avatar_url ?? null
  );

  const [profileSaving, setProfileSaving] = useState(false);
  const [orgSaving, setOrgSaving] = useState(false);
  const [avatarUploading, setAvatarUploading] = useState(false);

  // Hidden file input for avatar selection
  const fileInputRef = useRef<HTMLInputElement>(null);

  // ------------------------------------------------------------------
  // Profile save – PATCH /auth/me
  // ------------------------------------------------------------------
  async function handleProfileSubmit(e: React.FormEvent): Promise<void> {
    e.preventDefault();
    setProfileSaving(true);

    try {
      const { data: updatedUser } = await apiClient.patch<AuthUser>(
        "/auth/me",
        { full_name: fullName }
      );

      // Persist the fresh user object in the global auth store so the rest of
      // the application reflects the change without a reload.
      useAuthStore.getState().setUser(updatedUser);

      toast.success("Profile saved successfully.");
    } catch (err: unknown) {
      const message =
        err instanceof Error ? err.message : "Failed to save profile.";
      toast.error(message);
    } finally {
      setProfileSaving(false);
    }
  }

  // ------------------------------------------------------------------
  // Organisation save – PUT /organizations/{orgId}
  // ------------------------------------------------------------------
  async function handleOrgSubmit(e: React.FormEvent): Promise<void> {
    e.preventDefault();

    if (!user?.org_id) {
      toast.error("No organisation linked to your account.");
      return;
    }

    setOrgSaving(true);

    try {
      await apiClient.put(`/organizations/${user.org_id}`, { name: orgName });
      toast.success("Organisation name updated.");
    } catch (err: unknown) {
      const message =
        err instanceof Error ? err.message : "Failed to update organisation.";
      toast.error(message);
    } finally {
      setOrgSaving(false);
    }
  }

  // ------------------------------------------------------------------
  // Avatar upload – POST /uploads/avatar then PATCH /auth/me
  // ------------------------------------------------------------------
  async function handleAvatarChange(
    e: React.ChangeEvent<HTMLInputElement>
  ): Promise<void> {
    const file = e.target.files?.[0];
    if (!file) return;

    setAvatarUploading(true);

    try {
      // Step 1: upload the file as multipart form data
      const formData = new FormData();
      formData.append("file", file);

      const { data: uploadData } = await apiClient.post<{ url: string }>(
        "/uploads/avatar",
        formData,
        {
          headers: {
            // Let the browser set the correct multipart boundary automatically
            "Content-Type": "multipart/form-data",
          },
        }
      );

      const newAvatarUrl = uploadData.url;

      // Step 2: associate the uploaded URL with the current user profile
      const { data: updatedUser } = await apiClient.patch<AuthUser>(
        "/auth/me",
        { avatar_url: newAvatarUrl }
      );

      // Reflect the change locally and in the global store
      setAvatarUrl(newAvatarUrl);
      useAuthStore.getState().setUser(updatedUser);

      toast.success("Avatar updated.");
    } catch (err: unknown) {
      const message =
        err instanceof Error ? err.message : "Failed to upload avatar.";
      toast.error(message);
    } finally {
      setAvatarUploading(false);
      // Reset the file input so the same file can be re-selected if needed
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
      }
    }
  }

  // Derive the avatar initial letter from the current full name
  const avatarInitial = fullName ? fullName.charAt(0).toUpperCase() : "?";

  return (
    <div className="mx-auto max-w-2xl space-y-10">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Settings</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Manage your profile and organisation preferences.
        </p>
      </div>

      {/* ------------------------------------------------------------------ */}
      {/* User profile section                                                */}
      {/* ------------------------------------------------------------------ */}
      <section className="rounded-xl border border-border bg-card p-6">
        <h2 className="text-base font-semibold">User Profile</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Update your personal information.
        </p>

        <form onSubmit={handleProfileSubmit} className="mt-6 space-y-5">
          {/* Avatar row */}
          <div className="flex items-center gap-4">
            {/* Avatar display: show the uploaded image when available, otherwise
                fall back to the initials placeholder */}
            {avatarUrl ? (
              <img
                src={avatarUrl}
                alt={fullName || "Avatar"}
                className="h-16 w-16 rounded-full object-cover"
              />
            ) : (
              <div className="flex h-16 w-16 items-center justify-center rounded-full bg-primary/10 text-2xl font-bold text-primary">
                {avatarInitial}
              </div>
            )}

            <div className="flex flex-col gap-1">
              <p className="text-sm font-medium">{fullName || "Your Name"}</p>
              <p className="text-xs text-muted-foreground">
                {email || "your@email.com"}
              </p>

              {/* Hidden file input – triggered programmatically */}
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                className="hidden"
                onChange={handleAvatarChange}
              />

              <button
                type="button"
                disabled={avatarUploading}
                onClick={() => fileInputRef.current?.click()}
                className="mt-1 inline-flex h-7 items-center rounded-md border border-input bg-background px-3 text-xs font-medium hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {avatarUploading ? "Uploading..." : "Change Avatar"}
              </button>
            </div>
          </div>

          {/* Full name */}
          <div className="space-y-2">
            <label htmlFor="full-name" className="text-sm font-medium">
              Full Name
            </label>
            <input
              id="full-name"
              type="text"
              value={fullName}
              onChange={(e) => setFullName(e.target.value)}
              placeholder="Your full name"
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          {/* Email (read-only; changes require a separate verification flow) */}
          <div className="space-y-2">
            <label htmlFor="email" className="text-sm font-medium">
              Email Address
            </label>
            <input
              id="email"
              type="email"
              value={email}
              disabled
              placeholder="you@company.com"
              className="flex h-10 w-full rounded-md border border-input bg-muted px-3 py-2 text-sm text-muted-foreground ring-offset-background focus-visible:outline-none"
            />
            <p className="text-xs text-muted-foreground">
              Email changes require identity verification. Contact support to
              update.
            </p>
          </div>

          <button
            type="submit"
            disabled={profileSaving}
            className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {profileSaving ? "Saving..." : "Save Profile"}
          </button>
        </form>
      </section>

      {/* ------------------------------------------------------------------ */}
      {/* Organisation section                                                */}
      {/* ------------------------------------------------------------------ */}
      <section className="rounded-xl border border-border bg-card p-6">
        <h2 className="text-base font-semibold">Organisation</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Update your organisation&apos;s display name.
        </p>

        <form onSubmit={handleOrgSubmit} className="mt-6 space-y-5">
          <div className="space-y-2">
            <label htmlFor="org-name" className="text-sm font-medium">
              Organisation Name
            </label>
            <input
              id="org-name"
              type="text"
              value={orgName}
              onChange={(e) => setOrgName(e.target.value)}
              placeholder="Acme Corp"
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          <button
            type="submit"
            disabled={orgSaving || !orgName.trim()}
            className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {orgSaving ? "Saving..." : "Save Organisation"}
          </button>
        </form>
      </section>
    </div>
  );
}
