import Link from "next/link";
import { CreateOrgForm, CreateRequisitionForm, OrgList } from "@/components/hire/HireConsole";
import { BriefcaseIcon } from "@/components/icons";
import { EmptyState, buttonClass } from "@/components/ui";
import { listHireOrgs, listRequisitions, type HireOrg, type Requisition } from "@/lib/hire";
import { currentViewer } from "@/lib/session";

export const metadata = { title: "Linked Hire · JobShout" };

export default async function HirePage() {
  const viewer = await currentViewer();
  if (!viewer) {
    return (
      <main className="mx-auto max-w-3xl px-4 py-12 sm:px-6">
        <h1 className="font-display text-4xl font-semibold text-ink">Linked Hire</h1>
        <p className="mt-3 text-mute">
          Org-scoped hiring: requisitions, shortlists and invitations. Contact details stay
          private until a candidate accepts.
        </p>
        <Link href="/login" className={`${buttonClass("primary", "md")} mt-8 inline-flex`}>
          Sign in to hire
        </Link>
      </main>
    );
  }

  let orgs: HireOrg[] = [];
  let enabled = true;
  let error: string | null = null;
  try {
    orgs = await listHireOrgs(viewer);
  } catch (e) {
    const msg = e instanceof Error ? e.message : "Unavailable";
    if (msg.includes("HIRE_ENABLED") || msg.includes("not enabled")) {
      enabled = false;
    } else {
      error = msg;
    }
  }

  if (!enabled) {
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

  const requisitionsByOrg: Record<string, Requisition[]> = {};
  await Promise.all(
    orgs.map(async (org) => {
      try {
        requisitionsByOrg[org.id] = await listRequisitions(viewer, org.id);
      } catch {
        requisitionsByOrg[org.id] = [];
      }
    }),
  );

  return (
    <main className="mx-auto max-w-4xl px-4 py-12 sm:px-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="font-display text-4xl font-semibold text-ink">Linked Hire</h1>
          <p className="mt-3 max-w-2xl text-mute">
            Requisitions stay org-scoped. Candidates never see notes or scorecards. Invite
            from Members — email/phone only after they accept.
          </p>
        </div>
        <Link href="/opportunities" className={buttonClass("secondary", "md")}>
          Candidate opportunities
        </Link>
      </div>
      {error ? <p className="mt-4 text-sm text-shout">{error}</p> : null}
      <div className="mt-10 grid gap-8 lg:grid-cols-2">
        <CreateOrgForm />
        <CreateRequisitionForm orgs={orgs} />
      </div>
      <section className="mt-12">
        <h2 className="font-display text-2xl font-semibold text-ink">Your organisations</h2>
        <div className="mt-4">
          <OrgList orgs={orgs} requisitionsByOrg={requisitionsByOrg} />
        </div>
      </section>
    </main>
  );
}
