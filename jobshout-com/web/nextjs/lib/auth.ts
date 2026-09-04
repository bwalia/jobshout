import type { NextAuthOptions } from "next-auth";
import AppleProvider from "next-auth/providers/apple";
import FacebookProvider from "next-auth/providers/facebook";
import GoogleProvider from "next-auth/providers/google";
import LinkedInProvider from "next-auth/providers/linkedin";

export type SocialProviderId = "google" | "facebook" | "apple" | "linkedin";

export type SocialProviderMeta = {
  id: SocialProviderId;
  label: string;
  configured: boolean;
};

function envPair(id: string, secret: string): boolean {
  return Boolean(process.env[id]?.trim() && process.env[secret]?.trim());
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
      async jwt({ token, account, profile }) {
        if (account) {
          token.provider = account.provider;
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
