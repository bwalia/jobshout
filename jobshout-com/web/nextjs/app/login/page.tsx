import Link from "next/link";
import { getServerSession } from "next-auth";
import { authOptions, socialProviders } from "@/lib/auth";
import { SocialSignInButtons } from "@/components/SocialSignInButtons";

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

export default async function LoginPage({
  searchParams,
}: {
  searchParams: SearchParams;
}) {
  const session = await getServerSession(authOptions());
  const providers = socialProviders();
  const anyConfigured = providers.some((p) => p.configured);
  const callbackUrl = searchParams.callbackUrl || "/jobs";
  const errorKey = searchParams.error;
  const errorMessage = errorKey
    ? ERROR_COPY[errorKey] ?? ERROR_COPY.Default
    : null;

  return (
    <div className="relative overflow-hidden">
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_top,_rgba(232,93,4,0.12),_transparent_55%),linear-gradient(180deg,#e8eef7_0%,#f7f9fc_45%,#e8eef7_100%)]"
      />
      <div className="relative mx-auto flex min-h-[70vh] max-w-lg flex-col justify-center px-6 py-16">
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-accent">
          Welcome
        </p>
        <h1 className="mt-3 font-display text-4xl tracking-tight">Sign in to JobShout</h1>
        <p className="mt-3 text-slate">
          Use your Google, Facebook, Apple, or LinkedIn account. Humans stay in control —
          agents never sign in for you.
        </p>

        {session?.user ? (
          <div className="mt-10 rounded-2xl border border-slate/15 bg-white/90 p-6 shadow-sm">
            <p className="text-sm text-slate">Signed in as</p>
            <p className="mt-1 text-lg font-semibold">
              {session.user.name || session.user.email || "Account"}
            </p>
            <Link
              href="/jobs"
              className="mt-6 inline-flex rounded-full bg-accent px-5 py-2.5 text-sm font-semibold text-white hover:brightness-110"
            >
              Continue to jobs
            </Link>
          </div>
        ) : (
          <div className="mt-10 rounded-2xl border border-slate/15 bg-white/90 p-6 shadow-sm backdrop-blur">
            {errorMessage && (
              <p className="mb-4 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">
                {errorMessage}
              </p>
            )}
            <SocialSignInButtons providers={providers} callbackUrl={callbackUrl} />
            {!anyConfigured && (
              <p className="mt-5 text-sm leading-relaxed text-slate">
                Social login is wired up, but OAuth app credentials are not set yet. Add
                provider keys to{" "}
                <code className="rounded bg-mist px-1.5 py-0.5 text-ink">
                  web/nextjs/.env.local
                </code>{" "}
                (see <code className="rounded bg-mist px-1.5 py-0.5 text-ink">.env.example</code>
                ), then restart <code className="rounded bg-mist px-1.5 py-0.5 text-ink">make web</code>.
              </p>
            )}
          </div>
        )}

        <p className="mt-8 text-center text-sm text-slate">
          <Link href="/" className="font-medium text-ink hover:underline">
            ← Back to JobShout.com
          </Link>
        </p>
      </div>
    </div>
  );
}
