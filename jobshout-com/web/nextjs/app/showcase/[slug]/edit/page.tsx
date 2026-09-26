import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { ArrowLeftIcon } from "@/components/icons";
import { SignInPrompt } from "@/components/insights/SignInPrompt";
import { AppForm } from "@/components/showcase/AppForm";
import { ErrorNotice } from "@/components/ui";
import { isEditor } from "@/lib/insights";
import { KINDS, STATUS_LABELS, entryHref, getApp } from "@/lib/showcase";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = { title: "Edit listing", robots: { index: false } };

export default async function EditAppPage({ params }: { params: { slug: string } }) {
  const viewer = await currentViewer();
  if (!viewer) {
    return (
      <div className="mx-auto max-w-3xl px-5 pb-24 pt-14 sm:px-8">
        <SignInPrompt next={`/showcase/${params.slug}/edit`} title="Sign in to edit" body="Only the creator and the editors can change a listing." />
      </div>
    );
  }
  const [app, editor] = await Promise.all([getApp(params.slug, viewer), isEditor(viewer)]);
  if (!app) notFound();
  const owner = app.creator_email?.toLowerCase() === viewer.email.toLowerCase();

  return (
    <div className="mx-auto max-w-3xl px-5 pb-24 pt-8 sm:px-8 sm:pt-12">
      <Link
        href={entryHref(app)}
        className="inline-flex min-h-[44px] items-center gap-2 text-sm font-medium text-mute transition-colors duration-200 hover:text-ink"
      >
        <ArrowLeftIcon className="h-4 w-4" />
        Back to the {KINDS[app.kind].label.toLowerCase()}
      </Link>
      <h1 className="mt-4 font-display text-3xl font-semibold tracking-[-0.03em] text-ink sm:text-4xl">
        Edit: {app.name}
      </h1>
      <p className="mt-2 text-sm text-mute">Status: {STATUS_LABELS[app.status]}</p>
      {app.review_note ? (
        <div className="mt-6">
          <ErrorNotice title="An editor asked for changes" body={app.review_note} />
        </div>
      ) : null}

      <div className="mt-8">
        {!owner && !editor ? (
          <ErrorNotice title="You can only edit your own listings." />
        ) : app.status === "archived" && !editor ? (
          <ErrorNotice title="This was archived" body="Archived listings can only be changed by the editors." />
        ) : (
          <AppForm isStaff={editor} initial={app} />
        )}
      </div>
    </div>
  );
}
