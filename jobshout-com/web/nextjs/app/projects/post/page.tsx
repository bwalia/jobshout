"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";
import { useSession } from "next-auth/react";
import { Button, ErrorNotice, Field, Input, Textarea } from "@/components/ui";
import { createProject, type ProjectType } from "@/lib/projects";

export default function PostProjectPage() {
  const router = useRouter();
  const { data: session, status } = useSession();
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError(null);
    if (!session?.user?.email) {
      setError("Sign in to post a project.");
      return;
    }
    const fd = new FormData(e.currentTarget);
    const title = String(fd.get("title") ?? "").trim();
    const description = String(fd.get("description") ?? "").trim();
    const summary = String(fd.get("summary") ?? "").trim();
    const project_type = String(fd.get("project_type") ?? "fixed_price") as ProjectType;
    const budget_min = Number(fd.get("budget_min")) || null;
    const budget_max = Number(fd.get("budget_max")) || null;
    const skills = String(fd.get("skills") ?? "")
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    if (title.length < 8 || description.length < 40) {
      setError("Add a clear title and a longer description.");
      return;
    }
    setPending(true);
    try {
      const project = await createProject(
        { email: session.user.email, name: session.user.name ?? null },
        {
          title,
          description,
          summary: summary || description.slice(0, 160),
          project_type,
          budget_min,
          budget_max,
          currency: "GBP",
          skills_required: skills,
          deliverables: String(fd.get("deliverables") ?? "")
            .split("\n")
            .map((s) => s.trim())
            .filter(Boolean),
          duration_estimate: String(fd.get("duration") ?? "").trim() || undefined,
        },
      );
      router.push(`/projects/${project.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create project");
      setPending(false);
    }
  }

  if (status === "loading") {
    return <p className="mx-auto max-w-xl px-5 py-16 text-mute">Loading…</p>;
  }

  return (
    <div className="mx-auto max-w-xl px-5 py-10 sm:px-8">
      <h1 className="font-display text-3xl font-semibold tracking-[-0.03em] text-ink">Post a project</h1>
      <p className="mt-2 text-mute">Describe the work, budget and skills. Proposals stay free until credits are enabled.</p>
      <form onSubmit={onSubmit} className="mt-8 space-y-5">
        {error ? <ErrorNotice title={error} /> : null}
        <Field label="Title" htmlFor="title">
          <Input id="title" name="title" required maxLength={160} />
        </Field>
        <Field label="Summary" htmlFor="summary">
          <Input id="summary" name="summary" maxLength={240} />
        </Field>
        <Field label="Description" htmlFor="description">
          <Textarea id="description" name="description" required rows={8} />
        </Field>
        <Field label="Type" htmlFor="project_type">
          <select
            id="project_type"
            name="project_type"
            className="w-full rounded-xl border border-line bg-bg px-3 py-2 text-ink"
            defaultValue="fixed_price"
          >
            <option value="fixed_price">Fixed price</option>
            <option value="hourly">Hourly</option>
            <option value="retainer">Retainer</option>
            <option value="milestone">Milestone</option>
          </select>
        </Field>
        <div className="grid grid-cols-2 gap-4">
          <Field label="Budget min" htmlFor="budget_min">
            <Input id="budget_min" name="budget_min" type="number" min={0} step="1" />
          </Field>
          <Field label="Budget max" htmlFor="budget_max">
            <Input id="budget_max" name="budget_max" type="number" min={0} step="1" />
          </Field>
        </div>
        <Field label="Skills (comma-separated)" htmlFor="skills">
          <Input id="skills" name="skills" placeholder="Rust, Axum, Postgres" />
        </Field>
        <Field label="Duration" htmlFor="duration">
          <Input id="duration" name="duration" placeholder="2–4 weeks" />
        </Field>
        <Field label="Deliverables (one per line)" htmlFor="deliverables">
          <Textarea id="deliverables" name="deliverables" rows={4} />
        </Field>
        <Button type="submit" disabled={pending || !session?.user?.email}>
          {pending ? "Posting…" : "Post project"}
        </Button>
      </form>
    </div>
  );
}
