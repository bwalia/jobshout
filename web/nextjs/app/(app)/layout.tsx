"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { Sidebar } from "@/components/layout/Sidebar";
import { Topbar } from "@/components/layout/Topbar";
import { useAuthStore } from "@/lib/store/auth-store";
import { fetchCurrentUser } from "@/lib/auth/auth";

export default function AppLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const { isAuthenticated, isLoading, setUser, setLoading } = useAuthStore();

  // Rehydrate auth state on first render by checking the API.
  // The access token is already attached via the axios interceptor in
  // lib/api/client.ts, so this call will succeed if the token is valid.
  useEffect(() => {
    async function hydrateUser() {
      setLoading(true);
      const user = await fetchCurrentUser();
      if (user) {
        setUser(user);
      } else {
        setUser(null);
        router.replace("/login");
      }
    }

    // Only run when we haven't yet determined authentication state
    if (!isAuthenticated && !isLoading) {
      hydrateUser();
    } else if (isLoading) {
      hydrateUser();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // While we are resolving auth state, render a neutral loading screen
  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="flex flex-col items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          <p className="text-sm text-muted-foreground">Loading…</p>
        </div>
      </div>
    );
  }

  // Guard: push to login if not authenticated (redirect also fires in useEffect
  // above, but this prevents a flash of the shell).
  if (!isAuthenticated) {
    return null;
  }

  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <Sidebar />
      <div className="flex flex-1 flex-col overflow-hidden">
        <Topbar />
        <main className="flex-1 overflow-y-auto p-6">{children}</main>
      </div>
    </div>
  );
}
