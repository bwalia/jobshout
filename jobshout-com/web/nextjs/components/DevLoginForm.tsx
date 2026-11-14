"use client";

import { useState } from "react";
import { signIn } from "next-auth/react";
import { DEV_LOGIN_PROVIDER_ID } from "@/lib/auth";
import { buttonClass } from "@/components/ui";

type Props = {
  callbackUrl: string;
};

export function DevLoginForm({ callbackUrl }: Props) {
  const [email, setEmail] = useState("ada@example.com");
  const [name, setName] = useState("Ada Lovelace");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setPending(true);
    setError(null);
    const result = await signIn(DEV_LOGIN_PROVIDER_ID, {
      email: email.trim(),
      name: name.trim(),
      callbackUrl,
      redirect: false,
    });
    setPending(false);
    if (result?.error) {
      setError("Dev sign-in failed. Is AUTH_DEV_LOGIN still enabled?");
      return;
    }
    window.location.href = result?.url || callbackUrl;
  }

  return (
    <form onSubmit={onSubmit} className="mt-6 space-y-3 rounded-xl border border-dashed border-line bg-raised/60 p-4">
      <p className="text-xs font-semibold uppercase tracking-wide text-mute">
        Local / dev login
      </p>
      <p className="text-xs leading-relaxed text-mute">
        No OAuth secrets required. Enabled only when{" "}
        <code className="font-mono text-ink">AUTH_DEV_LOGIN=true</code> and{" "}
        <code className="font-mono text-ink">APP_ENV=local</code>. Never use on int/prod.
      </p>
      <label className="block text-xs font-medium text-mute">
        Email
        <input
          type="email"
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink"
          autoComplete="username"
        />
      </label>
      <label className="block text-xs font-medium text-mute">
        Display name
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink"
          autoComplete="name"
        />
      </label>
      {error ? <p className="text-xs text-shout">{error}</p> : null}
      <button type="submit" disabled={pending} className={buttonClass("primary", "md", "w-full")}>
        {pending ? "Signing in…" : "Sign in as local user"}
      </button>
    </form>
  );
}
