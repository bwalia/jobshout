"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { AppShell } from "@/components/layout/AppShell";
import { useAuthStore } from "@/lib/store/auth-store";
import { fetchCurrentUser } from "@/lib/auth/auth";

export default function AppLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const { isAuthenticated, isLoading, setUser, setLoading } = useAuthStore();

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

    if (!isAuthenticated && !isLoading) {
      void hydrateUser();
    } else if (isLoading) {
      void hydrateUser();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="flex flex-col items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          <p className="text-sm text-muted-foreground">Loading...</p>
        </div>
      </div>
    );
  }

  if (!isAuthenticated) {
    return null;
  }

  return <AppShell>{children}</AppShell>;
}
