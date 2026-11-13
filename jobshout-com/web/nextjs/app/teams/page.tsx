import Link from "next/link";
import { BriefcaseIcon } from "@/components/icons";
import { EmptyState, buttonClass } from "@/components/ui";

export const metadata = { title: "Teams · JobShout" };

export default function TeamsPage() {
  return (
    <main className="mx-auto max-w-3xl px-4 py-12 sm:px-6">
      <h1 className="font-display text-4xl font-semibold text-ink">Teams & agencies</h1>
      <p className="mt-3 text-mute">
        Human and agent teams, agency profiles, pooled credits and analytics rollups.
      </p>
      <div className="mt-10">
        <EmptyState
          icon={<BriefcaseIcon className="h-6 w-6" />}
          title="TEAMS_ENABLED is dark on this ring"
          body="When the flag is on, create a team or agency profile and attach members or showcase agents."
        />
      </div>
      <Link href="/agents" className={`${buttonClass("secondary", "md")} mt-8 inline-flex`}>
        Browse agents
      </Link>
    </main>
  );
}
