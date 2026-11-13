import Link from "next/link";
import { fetchPricing, formatPrice } from "@/lib/credits";
import { buttonClass } from "@/components/ui";

export const metadata = { title: "Pricing · JobShout" };

export default async function PricingPage() {
  const pricing = await fetchPricing();
  const packs = pricing?.packs ?? [];
  const plans = pricing?.plans ?? [];
  const purchases = pricing?.purchases_enabled ?? false;
  const creditsOn = pricing?.credits_enabled ?? false;

  return (
    <main className="mx-auto max-w-5xl px-4 py-12 sm:px-6">
      <h1 className="font-display text-4xl font-semibold text-ink">Pricing</h1>
      <p className="mt-3 max-w-2xl text-mute">
        Bid credits keep proposals meaningful. Everyone gets{" "}
        {pricing?.free_monthly_credits ?? 10} free credits each month when credits are on.
        Purchases stay off until the marketplace has enough open projects.
      </p>

      <section className="mt-10">
        <h2 className="font-display text-2xl font-semibold text-ink">Bid cost bands</h2>
        <ul className="mt-4 grid gap-3 sm:grid-cols-2">
          {(pricing?.bid_bands ?? []).map((b) => (
            <li key={`${b.credits}-${b.max_budget}`} className="border-t border-line pt-3">
              <p className="text-lg font-semibold text-ink">{b.credits} credit{b.credits === 1 ? "" : "s"}</p>
              <p className="text-sm text-mute">
                {b.max_budget == null
                  ? "Budgets over £10,000"
                  : b.max_budget <= 500
                    ? "Under £500"
                    : `Up to £${b.max_budget.toLocaleString()}`}
              </p>
            </li>
          ))}
        </ul>
      </section>

      <section className="mt-12">
        <h2 className="font-display text-2xl font-semibold text-ink">Credit packs</h2>
        {!purchases ? (
          <p className="mt-2 text-sm text-mute">
            Pack checkout is disabled on this ring (PURCHASES_ENABLED off).
          </p>
        ) : null}
        <ul className="mt-6 grid gap-6 sm:grid-cols-3">
          {packs.map((p) => (
            <li key={p.slug} className="border-t border-line pt-4">
              <h3 className="font-display text-xl font-semibold text-ink">{p.name}</h3>
              <p className="mt-1 text-2xl font-semibold text-ink">{p.credits} credits</p>
              <p className="text-mute">{formatPrice(p.price_cents, p.currency)}</p>
            </li>
          ))}
        </ul>
      </section>

      <section className="mt-12">
        <h2 className="font-display text-2xl font-semibold text-ink">Subscriptions</h2>
        <ul className="mt-6 grid gap-6 sm:grid-cols-3">
          {plans.map((p) => (
            <li key={p.slug} className="border-t border-line pt-4">
              <h3 className="font-display text-xl font-semibold text-ink">{p.name}</h3>
              <p className="mt-1 text-2xl font-semibold text-ink">
                {formatPrice(p.price_cents, p.currency)}
                <span className="text-base font-normal text-mute">/mo</span>
              </p>
              <p className="mt-2 text-sm text-mute">
                {p.monthly_credits > 0 ? `${p.monthly_credits} credits / month · ` : ""}
                {p.seats} seat{p.seats === 1 ? "" : "s"}
              </p>
            </li>
          ))}
        </ul>
      </section>

      <p className="mt-10 text-sm text-mute">
        Credits status: {creditsOn ? "enabled" : "dark on this ring"}.{" "}
        <Link href="/credits" className={buttonClass("secondary", "sm")}>
          Your credits
        </Link>
      </p>
    </main>
  );
}
