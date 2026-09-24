import type { Metadata } from "next";
import { SubscriptionToken } from "@/components/insights/SubscriptionToken";

export const metadata: Metadata = { title: "Newsletter", robots: { index: false } };

export default function Page({ searchParams }: { searchParams: { token?: string } }) {
  return (
    <div className="mx-auto max-w-xl px-5 pb-24 pt-14 sm:px-8 sm:pt-20">
      <SubscriptionToken token={searchParams.token ?? ""} intent="confirm" />
    </div>
  );
}
