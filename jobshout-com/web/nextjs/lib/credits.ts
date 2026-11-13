import { API_BASE, failure, headers, type Viewer } from "@/lib/insights";

export type SpendPreview = {
  cost: number;
  balance: number;
  balance_after: number;
  daily_bids: number;
  daily_cap: number;
};

export type CreditPack = {
  id: string;
  slug: string;
  name: string;
  credits: number;
  price_cents: number;
  currency: string;
};

export type SubscriptionPlan = {
  id: string;
  slug: string;
  name: string;
  monthly_credits: number;
  seats: number;
  price_cents: number;
  currency: string;
  features: string[];
};

export type PricingPayload = {
  packs: CreditPack[];
  plans: SubscriptionPlan[];
  free_monthly_credits: number;
  bid_bands: { max_budget: number | null; credits: number }[];
  credits_enabled: boolean;
  purchases_enabled: boolean;
};

export type CreditBalance = {
  account_id: string;
  balance: number;
  free: number;
  subscription: number;
  purchased: number;
  expiring_soon: number;
  plan: string;
};

export async function fetchPricing(): Promise<PricingPayload | null> {
  try {
    const res = await fetch(`${API_BASE}/api/v1/pricing`, { next: { revalidate: 60 } });
    if (!res.ok) return null;
    const body = (await res.json()) as { data: PricingPayload };
    return body.data;
  } catch {
    return null;
  }
}

export async function previewBidSpend(
  viewer: Viewer,
  projectId: string,
): Promise<SpendPreview | null> {
  try {
    const res = await fetch(`${API_BASE}/api/v1/credits/preview-bid/${projectId}`, {
      headers: headers(viewer),
      cache: "no-store",
    });
    if (res.status === 404) return null;
    if (!res.ok) throw await failure(res, "Could not preview bid cost");
    const body = (await res.json()) as { data: SpendPreview };
    return body.data;
  } catch {
    return null;
  }
}

export async function fetchBalance(viewer: Viewer): Promise<CreditBalance | null> {
  try {
    const res = await fetch(`${API_BASE}/api/v1/credits/balance`, {
      headers: headers(viewer),
      cache: "no-store",
    });
    if (res.status === 404) return null;
    if (!res.ok) throw await failure(res, "Could not load credit balance");
    const body = (await res.json()) as { data: CreditBalance };
    return body.data;
  } catch {
    return null;
  }
}

export function formatPrice(cents: number, currency: string): string {
  const amount = cents / 100;
  try {
    return new Intl.NumberFormat("en-GB", {
      style: "currency",
      currency: currency.toUpperCase(),
    }).format(amount);
  } catch {
    return `${currency.toUpperCase()} ${amount.toFixed(2)}`;
  }
}
