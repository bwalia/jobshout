import { getServerSession } from "next-auth";
import { authOptions } from "@/lib/auth";
import type { Viewer } from "@/lib/insights";

/** The signed-in user as the insights API expects them, or null. */
export async function currentViewer(): Promise<Viewer | null> {
  const session = await getServerSession(authOptions());
  const email = session?.user?.email;
  if (!email) return null;
  return { email, name: session.user?.name ?? null };
}
