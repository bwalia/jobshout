import { withAuth } from "next-auth/middleware";

/**
 * Signed-in routes for Phase 0. Public browse (jobs, showcase, insights,
 * members directory) stays open; anything that reads private PII requires auth.
 */
export default withAuth({
  pages: { signIn: "/login" },
});

export const config = {
  matcher: [
    "/applications",
    "/profile",
    "/profile/:path*",
    "/post-job",
    "/following",
    "/following/:path*",
    "/members/me",
    "/dashboard",
    "/dashboard/:path*",
    "/messages",
    "/messages/:path*",
    "/showcase/mine",
    "/showcase/new",
    "/showcase/review",
    "/insights/new",
    "/insights/mine",
    "/insights/review",
  ],
};
