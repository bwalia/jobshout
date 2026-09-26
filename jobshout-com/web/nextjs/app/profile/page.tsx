import Link from "next/link";
import type { Metadata } from "next";
import { getServerSession } from "next-auth";
import { ProfileForm } from "@/components/ProfileForm";
import { Badge, ErrorNotice } from "@/components/ui";
import { TargetIcon } from "@/components/icons";
import { getProfileByEmail, type CandidateProfile } from "@/lib/api";
import { authOptions } from "@/lib/auth";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Matching profile",
  description: "The profile your Career Agent uses to rank open roles for you.",
};

export default async function ProfilePage({
  searchParams,
}: {
  searchParams: { email?: string };
}) {
  const session = await getServerSession(authOptions());
  const emailHint = searchParams.email || session?.user?.email || "";

  let existing: CandidateProfile | null = null;
  let error = "";
  if (emailHint) {
    try {
      existing = await getProfileByEmail(emailHint);
    } catch (e) {
      error = e instanceof Error ? e.message : "Could not load your profile";
    }
  }

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <div className="grid gap-10 lg:grid-cols-[1fr_18rem] lg:gap-14">
        <div className="min-w-0 order-2 lg:order-1">
          <header className="max-w-2xl">
            <Badge tone="brand">
              <TargetIcon className="h-3.5 w-3.5" />
              For candidates
            </Badge>
            <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
              Matching profile
            </h1>
            <p className="mt-4 text-base leading-relaxed text-mute">
              Fill this in once. Your Career Agent uses it to rank every open role, with the
              reasoning attached — and nothing is sent anywhere without you.
            </p>
            {!session ? (
              <p className="mt-4 text-sm text-mute">
                <Link
                  href="/login?callbackUrl=%2Fprofile"
                  className="font-medium text-ink underline decoration-shout/50 underline-offset-4 transition-colors duration-200 hover:text-shout"
                >
                  Sign in
                </Link>{" "}
                to prefill your name and email.
              </p>
            ) : null}
          </header>

          {error ? (
            <div className="mt-8">
              <ErrorNotice title={error} body="You can still fill the form in and save." />
            </div>
          ) : null}

          {existing ? (
            <p
              role="status"
              className="mt-8 rounded-card border border-line bg-raised px-4 py-3 text-sm text-body"
            >
              Loaded the profile saved for{" "}
              <span className="font-semibold text-ink">{existing.email}</span>. Editing updates it.
            </p>
          ) : null}

          <div className="mt-9">
            <ProfileForm
              initial={existing}
              defaultEmail={emailHint}
              defaultName={session?.user?.name ?? ""}
            />
          </div>
        </div>

        <aside className="order-1 lg:order-2 lg:sticky lg:top-24 lg:self-start">
          <div className="surface-card p-5">
            <h2 className="font-display text-base font-semibold text-ink">
              What makes matching work
            </h2>
            <ul className="mt-4 space-y-3.5 text-sm leading-relaxed text-mute">
              {[
                "Skills carry the most weight — list the real ones, not aspirational ones.",
                "A salary floor filters out roles that would waste your evening.",
                "Matching notes are hard constraints. The agent respects them.",
                "Everything here stays private until you apply to something.",
              ].map((tip) => (
                <li key={tip} className="flex gap-3">
                  <span aria-hidden className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-shout" />
                  <span>{tip}</span>
                </li>
              ))}
            </ul>
          </div>
        </aside>
      </div>
    </div>
  );
}
