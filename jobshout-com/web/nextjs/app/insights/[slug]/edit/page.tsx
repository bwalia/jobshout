import Link from "next/link";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { ArrowLeftIcon } from "@/components/icons";
import { InsightForm } from "@/components/insights/InsightForm";
import { SignInPrompt } from "@/components/insights/SignInPrompt";
import { ErrorNotice } from "@/components/ui";
import { STATUS_LABELS, getInsight, isEditor, listTopics } from "@/lib/insights";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = { title: "Edit insight", robots: { index: false } };

export default async function EditInsightPage({ params }: { params: { slug: string } }) {
  const viewer = await currentViewer();
  if (!viewer) {
    return (
      <div className="mx-auto max-w-3xl px-5 pb-24 pt-14 sm:px-8">
        <SignInPrompt next={`/insights/${params.slug}/edit`} title="Sign in to edit" body="Only the author and the editors can change an insight." />
      </div>
    );
  }
  const [item, editor, topics] = await Promise.all([
    getInsight(params.slug, viewer),
    isEditor(viewer),
    listTopics(),
  ]);
  if (!item) notFound();
  const owner = item.author_email?.toLowerCase() === viewer.email.toLowerCase();
  const locked = !editor && (item.status === "published" || item.status === "archived");

  return (
    <div className="mx-auto max-w-3xl px-5 pb-24 pt-8 sm:px-8 sm:pt-12">
      <Link
        href={`/insights/${item.slug}`}
        className="inline-flex min-h-[44px] items-center gap-2 text-sm font-medium text-mute transition-colors duration-200 hover:text-ink"
      >
        <ArrowLeftIcon className="h-4 w-4" />
        Back to the insight
      </Link>
      <h1 className="mt-4 font-display text-3xl font-semibold tracking-[-0.03em] text-ink sm:text-4xl">
        Edit: {item.title}
      </h1>
      <p className="mt-2 text-sm text-mute">Status: {STATUS_LABELS[item.status]}</p>
      {item.review_note ? (
        <div className="mt-6">
          <ErrorNotice title="An editor asked for changes" body={item.review_note} />
        </div>
      ) : null}

      <div className="mt-8">
        {!owner && !editor ? (
          <ErrorNotice title="You can only edit your own insights." />
        ) : locked ? (
          <ErrorNotice
            title="This is already published"
            body="Published insights are changed by the editors. Email them with the correction and a source."
          />
        ) : (
          <InsightForm topics={topics} isStaff={editor} initial={item} />
        )}
      </div>
    </div>
  );
}
