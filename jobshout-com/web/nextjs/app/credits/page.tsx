import Link from "next/link";
import { redirect } from "next/navigation";
import { BriefcaseIcon } from "@/components/icons";
import { buttonClass, EmptyState } from "@/components/ui";
import { fetchBalance } from "@/lib/credits";
import { currentViewer } from "@/lib/session";

export const metadata = { title: "Credits · JobShout" };

export default async function CreditsPage() {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=/credits");

  const balance = await fetchBalance(viewer);

  if (!balance) {
    return (
      <main className="mx-auto max-w-3xl px-4 py-16 sm:px-6">
        <EmptyState
          icon={<BriefcaseIcon className="h-6 w-6" />}
          title="Credits are dark on this ring"
          body="When CREDITS_ENABLED is on, your free monthly grant and spend history appear here."
        />
        <Link href="/pricing" className={`${buttonClass("secondary", "md")} mt-6 inline-flex`}>
          View pricing
        </Link>
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-3xl px-4 py-12 sm:px-6">
      <h1 className="font-display text-4xl font-semibold text-ink">Credits</h1>
      <p className="mt-2 text-mute">Plan: {balance.plan}</p>
      <p className="mt-8 text-5xl font-semibold text-ink">{balance.balance}</p>
      <p className="text-mute">available</p>
      <dl className="mt-8 grid grid-cols-3 gap-4 border-t border-line pt-6 text-sm">
        <div>
          <dt className="text-mute">Free</dt>
          <dd className="text-lg font-semibold text-ink">{balance.free}</dd>
        </div>
        <div>
          <dt className="text-mute">Subscription</dt>
          <dd className="text-lg font-semibold text-ink">{balance.subscription}</dd>
        </div>
        <div>
          <dt className="text-mute">Purchased</dt>
          <dd className="text-lg font-semibold text-ink">{balance.purchased}</dd>
        </div>
      </dl>
      {balance.expiring_soon > 0 ? (
        <p className="mt-4 text-sm text-mute">
          {balance.expiring_soon} credits expire within 14 days.
        </p>
      ) : null}
      <div className="mt-8 flex flex-wrap gap-3">
        <Link href="/pricing" className={buttonClass("secondary", "md")}>
          Pricing
        </Link>
        <Link href="/projects" className={buttonClass("primary", "md")}>
          Browse projects
        </Link>
      </div>
    </main>
  );
}
