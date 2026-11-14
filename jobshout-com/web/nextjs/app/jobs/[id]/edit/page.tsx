import Link from "next/link";
import type { Metadata } from "next";
import { notFound, redirect } from "next/navigation";
import { PostJobForm } from "@/components/PostJobForm";
import { ErrorNotice, buttonClass } from "@/components/ui";
import { getJob } from "@/lib/api";
import { jobToFormValues } from "@/lib/job-form";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Edit job",
  robots: { index: false },
};

export default async function EditJobPage({ params }: { params: { id: string } }) {
  const viewer = await currentViewer();
  if (!viewer) redirect(`/login?callbackUrl=/jobs/${params.id}/edit`);

  // The API only returns an unpublished job to its poster, and the save is
  // checked again server-side, so this page cannot be used on someone
  // else's role.
  const job = await getJob(params.id, viewer);
  if (!job) notFound();

  const editable = job.status === "draft" || job.status === "published";

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header className="max-w-2xl">
        <Link href="/jobs/mine" className="text-sm font-medium text-mute hover:text-ink">
          ← My posted jobs
        </Link>
        <h1 className="mt-4 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
          Edit job
        </h1>
        <p className="mt-4 text-base leading-relaxed text-mute">
          {job.status === "published"
            ? "This role is live. Saved changes show on the board straight away."
            : "This is a draft. Save it again, or publish it when it is ready."}
        </p>
      </header>

      <div className="mt-10">
        {editable ? (
          <PostJobForm
            edit={{ jobId: job.id, status: job.status, values: jobToFormValues(job) }}
          />
        ) : (
          <div className="space-y-5">
            <ErrorNotice title={`This job is ${job.status}, so it can no longer be edited.`} />
            <Link href="/post-job" className={buttonClass("primary", "md")}>
              Post a new job
            </Link>
          </div>
        )}
      </div>
    </div>
  );
}
