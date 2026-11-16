import Link from "next/link";
import { buttonClass } from "@/components/ui";
import { currentViewer } from "@/lib/session";

export const metadata = { title: "Opportunities · JobShout" };

/**
 * Candidate-facing hiring invitations / opportunities (Linked Hire).
 * Contact details are never shown here — only the invite and accept/decline.
 */
export default async function OpportunitiesPage() {
  const viewer = await currentViewer();
  return (
    <main className="mx-auto max-w-3xl px-4 py-12 sm:px-6">
      <h1 className="font-display text-4xl font-semibold text-ink">Your opportunities</h1>
      <p className="mt-3 text-mute">
        Hiring invitations from Linked Hire land here. Your email and phone stay hidden until
        you accept.
      </p>
      {!viewer ? (
        <Link href="/login" className={`${buttonClass("primary", "md")} mt-8 inline-flex`}>
          Sign in
        </Link>
      ) : (
        <div className="mt-10 rounded-2xl border border-line bg-raised/40 p-6">
          <p className="text-sm text-body">
            No pending invitations yet. When a recruiter invites you, you will confirm before
            any contact details are shared.
          </p>
          <div className="mt-6 flex flex-wrap gap-3">
            <Link href="/dashboard" className={buttonClass("primary", "md")}>
              Open dashboard
            </Link>
            <Link href="/jobs" className={buttonClass("secondary", "md")}>
              Browse jobs
            </Link>
            <Link href="/projects" className={buttonClass("secondary", "md")}>
              Browse projects
            </Link>
          </div>
        </div>
      )}
    </main>
  );
}
