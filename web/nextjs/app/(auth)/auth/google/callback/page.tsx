"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { completeGoogleAuth } from "@/lib/auth/auth";
import { useAuthStore } from "@/lib/store/auth-store";

export default function GoogleCallbackPage() {
  const router = useRouter();
  const setUser = useAuthStore((s) => s.setUser);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const ticket = new URLSearchParams(window.location.search).get("ticket");
    if (!ticket) {
      setError("Missing sign-in ticket. Start again from the login page.");
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const data = await completeGoogleAuth(ticket);
        if (cancelled) return;
        setUser(data.user);
        router.replace("/chat");
      } catch {
        if (!cancelled) {
          setError("Google sign-in could not be completed. Try again.");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [router, setUser]);

  return (
    <div className="space-y-4 text-center">
      <h1 className="text-2xl font-bold tracking-tight text-foreground">
        Signing you in
      </h1>
      {error ? (
        <div className="space-y-3">
          <p className="rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {error}
          </p>
          <a
            href="/login"
            className="inline-flex text-sm font-medium text-primary hover:text-primary/80"
          >
            Back to sign in
          </a>
        </div>
      ) : (
        <div className="flex flex-col items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          <p className="text-sm text-muted-foreground">
            Finishing Google sign-in…
          </p>
        </div>
      )}
    </div>
  );
}
