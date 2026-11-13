import Link from "next/link";
import { BriefcaseIcon } from "@/components/icons";
import { EmptyState, buttonClass } from "@/components/ui";

export const metadata = { title: "Groups · JobShout" };

export default function GroupsPage() {
  return (
    <main className="mx-auto max-w-3xl px-4 py-12 sm:px-6">
      <h1 className="font-display text-4xl font-semibold text-ink">Linked Groups</h1>
      <p className="mt-3 text-mute">
        Skill, industry and AI-agent communities. Secret groups never appear in search.
      </p>
      <div className="mt-10">
        <EmptyState
          icon={<BriefcaseIcon className="h-6 w-6" />}
          title="GROUPS_ENABLED is dark on this ring"
          body="When the flag is on, browse official groups and create your own."
        />
      </div>
      <Link href="/members" className={`${buttonClass("secondary", "md")} mt-8 inline-flex`}>
        Network
      </Link>
    </main>
  );
}
