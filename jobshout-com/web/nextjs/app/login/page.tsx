import Link from "next/link";
import type { Metadata } from "next";
import { getServerSession } from "next-auth";
import { SocialSignInButtons } from "@/components/SocialSignInButtons";
import { ErrorNotice, buttonClass } from "@/components/ui";
import { CheckCircleIcon, ShieldIcon } from "@/components/icons";
import { authOptions, socialProviders } from "@/lib/auth";

export const metadata: Metadata = {
  title: "Sign in",
  description: "Sign in to JobShout.com with Google, Facebook, Apple, or LinkedIn.",
};

type SearchParams = { callbackUrl?: string; error?: string };

const ERROR_COPY: Record<string, string> = {
  OAuthSignin: "Could not start sign-in with that provider.",
  OAuthCallback: "The provider returned an error after sign-in.",
  OAuthCreateAccount: "Could not create an account from that provider.",
  Callback: "Sign-in callback failed.",
  AccessDenied: "Access was denied.",
  Configuration: "Sign-in is not configured on this server yet.",
  Default: "Something went wrong during sign-in.",
};

export default async function LoginPage({ searchParams }: { searchParams: SearchParams }) {
  const session = await getServerSession(authOptions());
  const providers = socialProviders();
  const anyConfigured = providers.some((p) => p.configured);
  const callbackUrl = searchParams.callbackUrl || "/jobs";
  const errorMessage = searchParams.error
    ? (ERROR_COPY[searchParams.error] ?? ERROR_COPY.Default)
    : null;

  return (
    <div className="relative overflow-hidden">
      <div aria-hidden className="pointer-events-none absolute inset-0 halo" />
      <div aria-hidden className="pointer-events-none absolute inset-0 grid-field" />

      <div className="relative mx-auto flex min-h-[calc(100vh-4.25rem)] max-w-md flex-col justify-center px-5 py-16 sm:px-8">
        <div className="surface-card p-7 shadow-card sm:p-9">
          {session?.user ? (
            <>
              <span className="flex h-12 w-12 items-center justify-center rounded-2xl bg-good/10 text-good">
                <CheckCircleIcon className="h-6 w-6" />
              </span>
              <h1 className="mt-5 font-display text-2xl font-semibold tracking-[-0.02em] text-ink">
                You are signed in
              </h1>
              <p className="mt-2 text-sm text-mute">
                as{" "}
                <span className="font-semibold text-ink">
                  {session.user.name || session.user.email || "your account"}
                </span>
              </p>
              <Link href={callbackUrl} className={buttonClass("primary", "md", "mt-7 w-full")}>
                Continue
              </Link>
              <Link
                href="/api/auth/signout"
                className={buttonClass("ghost", "md", "mt-2 w-full")}
              >
                Sign out
              </Link>
            </>
          ) : (
            <>
              <h1 className="font-display text-3xl font-semibold tracking-[-0.025em] text-ink">
                Sign in
              </h1>
              <p className="mt-3 text-sm leading-relaxed text-mute">
                Signing in prefills your applications and keeps your matches in one place. Agents
                never sign in for you.
              </p>

              {errorMessage ? (
                <div className="mt-6">
                  <ErrorNotice title={errorMessage} />
                </div>
              ) : null}

              <div className="mt-7">
                <SocialSignInButtons providers={providers} callbackUrl={callbackUrl} />
              </div>

              {!anyConfigured ? (
                <p className="mt-6 rounded-xl border border-line bg-raised px-4 py-3 text-xs leading-relaxed text-mute">
                  No OAuth credentials are set. Add provider keys to{" "}
                  <code className="font-mono text-ink">web/nextjs/.env.local</code> from{" "}
                  <code className="font-mono text-ink">.env.example</code>, then restart the app.
                </p>
              ) : null}

              <p className="mt-7 flex items-start gap-2.5 border-t border-line pt-6 text-xs leading-relaxed text-mute">
                <ShieldIcon className="mt-0.5 h-4 w-4 shrink-0 text-signal" />
                You can browse the whole board and apply without an account — signing in just
                saves you retyping.
              </p>
            </>
          )}
        </div>

        <p className="mt-6 text-center text-sm text-mute">
          <Link href="/" className="inline-flex min-h-[44px] items-center px-2 transition-colors duration-200 hover:text-shout">
            Back to JobShout.com
          </Link>
        </p>
      </div>
    </div>
  );
}
