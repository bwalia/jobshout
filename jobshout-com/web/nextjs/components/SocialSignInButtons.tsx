"use client";

import type { ReactElement } from "react";
import { signIn } from "next-auth/react";
import type { SocialProviderId } from "@/lib/auth";

const ICONS: Record<SocialProviderId, ReactElement> = {
  google: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" aria-hidden>
      <path
        fill="#EA4335"
        d="M12 10.2v3.6h5.1c-.2 1.2-.9 2.3-1.9 3l3.1 2.4c1.8-1.7 2.9-4.1 2.9-7 0-.7-.1-1.3-.2-1.9H12z"
      />
      <path
        fill="#34A853"
        d="M6.6 14.3l-.9.7-2.5 1.9C4.9 19.5 8.2 21.5 12 21.5c2.7 0 4.9-.9 6.5-2.4l-3.1-2.4c-.9.6-2 1-3.4 1-2.6 0-4.8-1.7-5.6-4.1z"
      />
      <path
        fill="#4A90E2"
        d="M3.2 7.1C2.4 8.7 2 10.3 2 12s.4 3.3 1.2 4.9l3.4-2.6C6.2 13.4 6 12.7 6 12c0-.7.2-1.4.5-2z"
      />
      <path
        fill="#FBBC05"
        d="M12 5.5c1.5 0 2.8.5 3.8 1.5l2.8-2.8C16.9 2.5 14.7 1.5 12 1.5 8.2 1.5 4.9 3.5 3.2 7.1l3.4 2.6C7.2 7.2 9.4 5.5 12 5.5z"
      />
    </svg>
  ),
  facebook: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" aria-hidden>
      <path
        fill="#1877F2"
        d="M24 12.1C24 5.4 18.6 0 12 0S0 5.4 0 12.1C0 18.1 4.4 23.1 10.1 24v-8.4H7.1V12h3V9.4c0-3 1.8-4.7 4.5-4.7 1.3 0 2.7.2 2.7.2v3h-1.5c-1.5 0-2 .9-2 1.9V12h3.4l-.5 3.6h-2.9V24C19.6 23.1 24 18.1 24 12.1z"
      />
    </svg>
  ),
  apple: (
    <svg viewBox="0 0 24 24" className="h-5 w-5 fill-ink" aria-hidden>
      <path d="M16.4 12.7c0-2.3 1.9-3.4 2-3.5-1.1-1.6-2.8-1.8-3.4-1.8-1.4-.2-2.8.9-3.5.9-.7 0-1.9-.8-3.1-.8-1.6 0-3.1 1-3.9 2.4-1.7 2.9-.4 7.2 1.2 9.6.8 1.1 1.7 2.4 3 2.4 1.2 0 1.6-.8 3.1-.8s1.8.8 3.1.8c1.3 0 2.1-1.1 2.9-2.2.9-1.3 1.3-2.5 1.3-2.6-.1 0-2.5-1-2.5-3.4zM14.2 5.6c.6-.8 1.1-1.9 1-3-.9 0-2.1.6-2.7 1.4-.6.7-1.2 1.8-1 2.9 1 .1 2.1-.5 2.7-1.3z" />
    </svg>
  ),
  linkedin: (
    <svg viewBox="0 0 24 24" className="h-5 w-5" aria-hidden>
      <path
        fill="#0A66C2"
        d="M20.5 2h-17A1.5 1.5 0 002 3.5v17A1.5 1.5 0 003.5 22h17a1.5 1.5 0 001.5-1.5v-17A1.5 1.5 0 0020.5 2zM8 19H5v-9h3zM6.5 8.3A1.8 1.8 0 116.5 4.8a1.8 1.8 0 010 3.5zM19 19h-3v-4.7c0-1.1 0-2.6-1.6-2.6s-1.8 1.2-1.8 2.5V19h-3v-9h2.9v1.2h.1c.4-.8 1.4-1.6 2.9-1.6 3.1 0 3.7 2 3.7 4.7V19z"
      />
    </svg>
  ),
};

type Props = {
  providers: { id: SocialProviderId; label: string; configured: boolean }[];
  callbackUrl: string;
};

export function SocialSignInButtons({ providers, callbackUrl }: Props) {
  return (
    <ul className="space-y-3">
      {providers.map((p) => (
        <li key={p.id}>
          <button
            type="button"
            disabled={!p.configured}
            onClick={() => {
              if (!p.configured) return;
              void signIn(p.id, { callbackUrl });
            }}
            className="group flex w-full items-center gap-3 border border-line bg-white px-4 py-3.5 text-left text-sm font-semibold text-ink transition enabled:hover:border-signal enabled:hover:bg-paper/80 disabled:cursor-not-allowed disabled:opacity-45"
          >
            <span className="flex h-9 w-9 items-center justify-center bg-paper">
              {ICONS[p.id]}
            </span>
            <span className="flex-1">{p.label}</span>
            {!p.configured && (
              <span className="text-xs font-medium text-mute">Add credentials</span>
            )}
          </button>
        </li>
      ))}
    </ul>
  );
}
