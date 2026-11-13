import Link from "next/link";
import { redirect } from "next/navigation";
import { BriefcaseIcon } from "@/components/icons";
import { EmptyState, buttonClass } from "@/components/ui";
import { currentViewer } from "@/lib/session";

export const metadata = { title: "AI drafts · JobShout" };

export default async function AiDraftsPage() {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=/ai");

  return (
    <main className="mx-auto max-w-3xl px-4 py-12 sm:px-6">
      <h1 className="font-display text-4xl font-semibold text-ink">AI assistants</h1>
      <p className="mt-3 text-mute">
        Proposal Generator, Project Breakdown and Talent Matchmaker run on the JobShout
        platform (`POST /tasks/launch`). Drafts never bid, post or hire — you confirm.
      </p>
      <EmptyState
        icon={<BriefcaseIcon className="h-6 w-6" />}
        title="AI_FEATURES is dark on this ring"
        body="When enabled with platform credentials, drafts appear in your approval inbox here and on the dashboard."
      />
      <div className="mt-8 flex flex-wrap gap-3">
        <Link href="/projects" className={buttonClass("secondary", "md")}>
          Projects
        </Link>
        <Link href="/dashboard" className={buttonClass("primary", "md")}>
          Dashboard
        </Link>
      </div>
    </main>
  );
}
