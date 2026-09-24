"use client";

import Link from "next/link";
import { useFormState, useFormStatus } from "react-dom";
import { EMPTY_FORM_STATE } from "@/app/actions";
import { confirmSubscriptionAction } from "@/app/insights/actions";
import { CheckCircleIcon, MailIcon } from "@/components/icons";
import { Button, ErrorNotice, buttonClass } from "@/components/ui";

function Submit({ label }: { label: string }) {
  const { pending } = useFormStatus();
  return (
    <Button type="submit" size="lg" disabled={pending}>
      {pending ? "Working…" : label}
    </Button>
  );
}

/**
 * Confirmation is a button press, not a page load: mail scanners open links
 * automatically, and that must not count as someone agreeing to subscribe.
 */
export function SubscriptionToken({ token, intent }: { token: string; intent: "confirm" | "unsubscribe" }) {
  const [state, action] = useFormState(confirmSubscriptionAction, EMPTY_FORM_STATE);
  const confirm = intent === "confirm";

  return (
    <div className="surface-card p-8 text-center sm:p-12">
      <span className={`mx-auto flex h-14 w-14 items-center justify-center rounded-2xl ${state.ok ? "bg-good/10 text-good" : "bg-raised text-shout"}`}>
        {state.ok ? <CheckCircleIcon className="h-7 w-7" /> : <MailIcon className="h-7 w-7" />}
      </span>
      <h1 className="mt-5 font-display text-2xl font-semibold text-ink sm:text-3xl">
        {state.ok ? state.message : confirm ? "Confirm your subscription" : "Unsubscribe"}
      </h1>
      <p className="mx-auto mt-3 max-w-md text-sm leading-relaxed text-mute">
        {state.ok
          ? confirm
            ? "The next weekly issue will land in your inbox."
            : "You will not get any more issues. You can resubscribe at any time."
          : confirm
            ? "One click and you will get the weekly Insights digest."
            : "Stop receiving the weekly Insights digest."}
      </p>
      {!token ? (
        <div className="mt-6 text-left">
          <ErrorNotice title="This link is missing its token." body="Open the link from the email again." />
        </div>
      ) : state.ok ? (
        <Link href="/insights" className={buttonClass("secondary", "md", "mt-7")}>
          Read Insights
        </Link>
      ) : (
        <form action={action} className="mt-7">
          <input type="hidden" name="token" value={token} />
          <input type="hidden" name="intent" value={intent} />
          <Submit label={confirm ? "Confirm subscription" : "Unsubscribe"} />
          {state.message ? (
            <div className="mt-5 text-left">
              <ErrorNotice title={state.message} />
            </div>
          ) : null}
        </form>
      )}
    </div>
  );
}
