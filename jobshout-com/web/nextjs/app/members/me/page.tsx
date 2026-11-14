import { redirect } from "next/navigation";
import { ErrorNotice } from "@/components/ui";
import { getMe, memberHref } from "@/lib/members";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

/** "My profile" links land here and go on to the signed-in member's page. */
export default async function MyMemberPage() {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=%2Fmembers%2Fme");

  let handle = "";
  try {
    handle = (await getMe(viewer)).handle;
  } catch {
    // Fall through to the notice below.
  }
  if (handle) redirect(memberHref(handle));

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-14 sm:px-8">
      <ErrorNotice
        title="Your member profile is not available right now"
        body="Member profiles may be switched off on this site, or the marketplace API did not answer."
      />
    </div>
  );
}
