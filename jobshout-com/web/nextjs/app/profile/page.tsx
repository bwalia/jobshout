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
    <div className="mx-auto max-w-3xl px-6 pb-20 pt-12">
      <h1 className="font-display text-4xl tracking-tight text-ink md:text-5xl">
        Matching profile
      </h1>
      <p className="mt-4 max-w-2xl leading-relaxed text-mute">
        Skills, preferred roles, and notes your Career Agent uses to rank open jobs — with
        explainable scores. Nothing is sent without you.
      </p>
      <p className="mt-3 text-sm text-mute">
        Optional:{" "}
        <Link href="/login?callbackUrl=%2Fprofile" className="text-ink underline decoration-signal/50 underline-offset-4 hover:text-signal">
          sign in
        </Link>{" "}
        to prefill your email.
      </p>

      <div className="mt-10 border border-line bg-white/70 p-6 sm:p-8">
        <ProfileForm
          initial={existing}
          defaultEmail={emailHint}
          defaultName={session?.user?.name ?? ""}
        />
      </div>
    </div>
  );
}
