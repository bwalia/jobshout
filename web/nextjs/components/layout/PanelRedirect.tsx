"use client";

import { useEffect } from "react";
import { useRouter, useSearchParams } from "next/navigation";

/** Sends an old panel URL to its new home, keeping its query parameters. */
export function PanelRedirect({ to }: { to: string }) {
  const router = useRouter();
  const params = useSearchParams();
  useEffect(() => {
    const extra = params.toString();
    router.replace(extra ? `${to}${to.includes("?") ? "&" : "?"}${extra}` : to);
  }, [to, params, router]);
  return null;
}
