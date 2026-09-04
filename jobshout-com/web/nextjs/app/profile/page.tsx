import Link from "next/link";
import { getServerSession } from "next-auth";
import { ProfileForm } from "@/components/ProfileForm";
import { authOptions } from "@/lib/auth";
import { getProfileByEmail } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function ProfilePage({
  searchParams,
}: {
  searchParams: { email?: string };
}) {
  const session = await getServerSession(authOptions());
  const emailHint = searchParams.email || session?.user?.email || "";
  let existing = null;
  if (emailHint) {
    try {
      existing = await getProfileByEmail(emailHint);
    } catch {
      existing = null;
    }
  }

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <p className="text-sm font-semibold uppercase tracking-[0.18em] text-accent">
        Career profile
      </p>
      <h1 className="mt-2 font-display text-4xl tracking-tight">Build your matching profile</h1>
      <p className="mt-3 max-w-2xl text-slate">
        Capture skills, preferred roles, and notes once. The Career matching agent uses this
        profile to rank open jobs with explainable scores — humans stay in control before any
        application is sent.
      </p>
      <p className="mt-4 text-sm text-slate">
        Prefer social sign-in first?{" "}
        <Link href="/login?callbackUrl=%2Fprofile" className="font-medium text-ink underline">
          Sign in
        </Link>
        , then return here — we will prefill your email.
      </p>

      <div className="mt-10 rounded-2xl border border-slate/15 bg-white/90 p-6 shadow-sm sm:p-8">
        <ProfileForm
          initial={existing}
          defaultEmail={emailHint}
          defaultName={session?.user?.name ?? ""}
        />
      </div>
    </div>
  );
}
