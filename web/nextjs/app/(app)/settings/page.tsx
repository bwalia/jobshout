"use client";

import { useState } from "react";
import { useAuthStore } from "@/lib/store/auth-store";

export default function SettingsPage() {
  const user = useAuthStore((s) => s.user);

  // Initialise form fields from the authenticated user; fall back to empty strings
  const [fullName, setFullName] = useState(user?.full_name ?? "");
  const [email, setEmail] = useState(user?.email ?? "");
  const [orgName, setOrgName] = useState("");

  const [profileSaving, setProfileSaving] = useState(false);
  const [profileSaved, setProfileSaved] = useState(false);
  const [orgSaving, setOrgSaving] = useState(false);
  const [orgSaved, setOrgSaved] = useState(false);

  async function handleProfileSubmit(e: React.FormEvent): Promise<void> {
    e.preventDefault();
    setProfileSaving(true);
    setProfileSaved(false);

    try {
      // TODO: call PATCH /api/v1/auth/me with { full_name: fullName }
      await new Promise((resolve) => setTimeout(resolve, 600));
      setProfileSaved(true);
      setTimeout(() => setProfileSaved(false), 3000);
    } finally {
      setProfileSaving(false);
    }
  }

  async function handleOrgSubmit(e: React.FormEvent): Promise<void> {
    e.preventDefault();
    setOrgSaving(true);
    setOrgSaved(false);

    try {
      // TODO: call PUT /api/v1/organizations/{orgID} with { name: orgName }
      await new Promise((resolve) => setTimeout(resolve, 600));
      setOrgSaved(true);
      setTimeout(() => setOrgSaved(false), 3000);
    } finally {
      setOrgSaving(false);
    }
  }

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
          {/* Avatar placeholder */}
          <div className="flex items-center gap-4">
            <div className="flex h-16 w-16 items-center justify-center rounded-full bg-primary/10 text-2xl font-bold text-primary">
              {fullName ? fullName.charAt(0).toUpperCase() : "?"}
            </div>
            <div>
              <p className="text-sm font-medium">{fullName || "Your Name"}</p>
              <p className="text-xs text-muted-foreground">{email || "your@email.com"}</p>
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
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@company.com"
              disabled
              className="flex h-10 w-full rounded-md border border-input bg-muted px-3 py-2 text-sm text-muted-foreground ring-offset-background focus-visible:outline-none"
            />
            <p className="text-xs text-muted-foreground">
              Email changes require identity verification. Contact support to update.
            </p>
          </div>

          <div className="flex items-center gap-3">
            <button
              type="submit"
              disabled={profileSaving}
              className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {profileSaving ? "Saving..." : "Save Profile"}
            </button>
            {profileSaved && (
              <span className="flex items-center gap-1 text-sm text-green-600">
                <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                </svg>
                Saved
              </span>
            )}
          </div>
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

          <div className="flex items-center gap-3">
            <button
              type="submit"
              disabled={orgSaving || !orgName.trim()}
              className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {orgSaving ? "Saving..." : "Save Organisation"}
            </button>
            {orgSaved && (
              <span className="flex items-center gap-1 text-sm text-green-600">
                <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                </svg>
                Saved
              </span>
            )}
          </div>
        </form>
      </section>
    </div>
  );
}
