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
  const errorMessage = errorKey ? ERROR_COPY[errorKey] ?? ERROR_COPY.Default : null;

  return (
    <div className="signal-field relative mx-auto flex min-h-[70vh] max-w-lg flex-col justify-center px-6 py-16">
      <h1 className="font-display text-4xl tracking-tight text-ink">Sign in</h1>
      <p className="mt-3 text-mute">
        Google, Facebook, Apple, or LinkedIn. Agents never sign in for you.
      </p>

      {session?.user ? (
        <div className="mt-10 border border-line bg-white/80 p-6">
          <p className="text-sm text-mute">Signed in as</p>
          <p className="mt-1 text-lg font-semibold text-ink">
            {session.user.name || session.user.email || "Account"}
          </p>
          <Link
            href="/jobs"
            className="mt-6 inline-flex bg-shout px-5 py-2.5 text-sm font-semibold text-white hover:brightness-110"
          >
            Continue to open roles
          </Link>
        </div>
      ) : (
        <div className="mt-10 border border-line bg-white/80 p-6">
          {errorMessage && (
            <p className="mb-4 border border-shout/30 bg-shout/5 px-3 py-2 text-sm text-ink">
              {errorMessage}
            </p>
          )}
          <SocialSignInButtons providers={providers} callbackUrl={callbackUrl} />
          {!anyConfigured && (
            <p className="mt-5 text-sm leading-relaxed text-mute">
              OAuth credentials are not set yet. Add provider keys to{" "}
              <code className="text-ink">web/nextjs/.env.local</code> from{" "}
              <code className="text-ink">.env.example</code>, then restart the web app.
            </p>
          )}
        </div>
      )}

      <p className="mt-8 text-center text-sm text-mute">
        <Link href="/" className="hover:text-signal">
          Back to JobShout.com
        </Link>
      </p>
    </div>
  );
}
