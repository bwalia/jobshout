import Link from "next/link";
import { BriefcaseIcon } from "@/components/icons";
import { EmptyState, buttonClass } from "@/components/ui";

export const metadata = { title: "Linked Hire · JobShout" };

export default function HirePage() {
  return (
    <main className="mx-auto max-w-3xl px-4 py-12 sm:px-6">
      <h1 className="font-display text-4xl font-semibold text-ink">Linked Hire</h1>
      <p className="mt-3 text-mute">
        Org-scoped hiring: requisitions, shortlists and invitations. Contact details stay
        private until a candidate accepts.
      </p>
      <div className="mt-10">
        <EmptyState
          icon={<BriefcaseIcon className="h-6 w-6" />}
          title="HIRE_ENABLED is dark on this ring"
          body="When the flag is on, create a hiring organisation and open requisitions from here."
        />
      </div>
      <Link href="/members" className={`${buttonClass("secondary", "md")} mt-8 inline-flex`}>
        Browse talent
      </Link>
    </main>
  );
}
