import type { NextAuthOptions } from "next-auth";
import AppleProvider from "next-auth/providers/apple";
import CredentialsProvider from "next-auth/providers/credentials";
import FacebookProvider from "next-auth/providers/facebook";
import GoogleProvider from "next-auth/providers/google";
import LinkedInProvider from "next-auth/providers/linkedin";

export type SocialProviderId = "google" | "facebook" | "apple" | "linkedin";

export type SocialProviderMeta = {
  id: SocialProviderId;
  label: string;
  configured: boolean;
};

/** Provider id for the local-only credentials sign-in (never on int/prod). */
export const DEV_LOGIN_PROVIDER_ID = "dev-login";

const PROD_LIKE_ENVS = new Set(["int", "test", "acc", "prod", "production", "staging"]);

function envPair(id: string, secret: string): boolean {
  return Boolean(process.env[id]?.trim() && process.env[secret]?.trim());
}

function envFlag(name: string): boolean {
  const v = (process.env[name] || "").trim().toLowerCase();
  return v === "1" || v === "true" || v === "yes" || v === "on";
}

/**
 * Local/dev credentials login. Requires AUTH_DEV_LOGIN=true and a non-ring
 * APP_ENV (local / development / empty). Never enables on int/test/acc/prod
 * even if someone sets the flag by mistake.
 */
export function isDevLoginEnabled(): boolean {
  if (!envFlag("AUTH_DEV_LOGIN")) return false;
  const appEnv = (process.env.APP_ENV || "").trim().toLowerCase();
  if (PROD_LIKE_ENVS.has(appEnv)) return false;
  // Allow empty APP_ENV only when Next is clearly in a local `next dev` run.
  if (!appEnv) return process.env.NODE_ENV !== "production";
  return appEnv === "local" || appEnv === "development" || appEnv === "dev";
}

/** Which social IdPs have credentials in the environment. */
export function socialProviders(): SocialProviderMeta[] {
  return [
    {
      id: "google",
      label: "Continue with Google",
      configured: envPair("GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET"),
    },
    {
      id: "facebook",
      label: "Continue with Facebook",
      configured: envPair("FACEBOOK_CLIENT_ID", "FACEBOOK_CLIENT_SECRET"),
    },
    {
      id: "apple",
      label: "Continue with Apple",
      configured: envPair("APPLE_ID", "APPLE_SECRET"),
    },
    {
      id: "linkedin",
      label: "Continue with LinkedIn",
      configured: envPair("LINKEDIN_CLIENT_ID", "LINKEDIN_CLIENT_SECRET"),
    },
  ];
}

export function authOptions(): NextAuthOptions {
  const providers = [];

  if (envPair("GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET")) {
    providers.push(
      GoogleProvider({
        clientId: process.env.GOOGLE_CLIENT_ID!,
        clientSecret: process.env.GOOGLE_CLIENT_SECRET!,
      }),
    );
  }

  if (envPair("FACEBOOK_CLIENT_ID", "FACEBOOK_CLIENT_SECRET")) {
    providers.push(
      FacebookProvider({
        clientId: process.env.FACEBOOK_CLIENT_ID!,
        clientSecret: process.env.FACEBOOK_CLIENT_SECRET!,
      }),
    );
  }

  if (envPair("APPLE_ID", "APPLE_SECRET")) {
    providers.push(
      AppleProvider({
        clientId: process.env.APPLE_ID!,
        clientSecret: process.env.APPLE_SECRET!,
      }),
    );
  }

  if (envPair("LINKEDIN_CLIENT_ID", "LINKEDIN_CLIENT_SECRET")) {
    providers.push(
      LinkedInProvider({
        clientId: process.env.LINKEDIN_CLIENT_ID!,
        clientSecret: process.env.LINKEDIN_CLIENT_SECRET!,
      }),
    );
  }

  if (isDevLoginEnabled()) {
    providers.push(
      CredentialsProvider({
        id: DEV_LOGIN_PROVIDER_ID,
        name: "Dev login",
        credentials: {
          email: { label: "Email", type: "email" },
          name: { label: "Name", type: "text" },
        },
        async authorize(credentials) {
          // Re-check at authorize time so a stale process cannot keep the door open.
          if (!isDevLoginEnabled()) return null;
          const email = (credentials?.email || "").trim().toLowerCase();
          if (!email || !email.includes("@") || email.length > 254) return null;
          const name =
            (credentials?.name || "").trim() || email.split("@")[0] || "Local user";
          return { id: email, email, name };
        },
      }),
    );
  }

  return {
    providers,
    pages: {
      signIn: "/login",
      error: "/login",
    },
    session: {
      strategy: "jwt",
    },
    secret: process.env.NEXTAUTH_SECRET,
    callbacks: {
      async jwt({ token, account, profile, user }) {
        if (account) {
          token.provider = account.provider;
        }
        if (user?.email) {
          token.email = user.email;
        }
        if (profile && "email" in profile && profile.email) {
          token.email = profile.email;
        }
        return token;
      },
      async session({ session, token }) {
        if (session.user) {
          session.user.email = (token.email as string | undefined) ?? session.user.email;
        }
        return session;
      },
    },
  };
}
