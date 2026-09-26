import type { Metadata } from "next";
import { PostJobForm } from "@/components/PostJobForm";
import { Badge } from "@/components/ui";
import { SparkIcon } from "@/components/icons";

export const metadata: Metadata = {
  title: "Post a job",
  description: "Publish a role to the JobShout.com board in about two minutes.",
};

export default function PostJobPage() {
  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header className="max-w-2xl">
        <Badge tone="brand">
          <SparkIcon className="h-3.5 w-3.5" />
          For employers
        </Badge>
        <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
          Post a job
        </h1>
        <p className="mt-4 text-base leading-relaxed text-mute">
          Three short sections. Publish straight to the board, or save a draft and come back to
          it. Everything you type appears in the preview as you go.
        </p>
      </header>

      <div className="mt-10">
        <PostJobForm />
      </div>
    </div>
  );
}
