"use client";

import { useRef, useState } from "react";
import { FieldHint, FieldLabel } from "@/components/ui/field-hint";
import { apiErrorMessage } from "@/lib/api/client";
import { addCareerBlacklist, uploadCareerCV, upsertCareerStory } from "@/lib/api/career";
import type { CareerBlacklistEntry, CareerStory } from "@/types/career";
import { toast } from "sonner";

export function CareerProfilePanel({
  cvDraft,
  setCvDraft,
  fullName,
  setFullName,
  sponsorship,
  setSponsorship,
  titles,
  setTitles,
  minComp,
  setMinComp,
  houseRules,
  setHouseRules,
  blacklist,
  stories,
  busy,
  saving,
  savedFlash,
  onSave,
  onFillFromCV,
  onReload,
  onError,
}: {
  cvDraft: string;
  setCvDraft: (v: string) => void;
  fullName: string;
  setFullName: (v: string) => void;
  sponsorship: boolean;
  setSponsorship: (v: boolean) => void;
  titles: string;
  setTitles: (v: string) => void;
  minComp: string;
  setMinComp: (v: string) => void;
  houseRules: string;
  setHouseRules: (v: string) => void;
  blacklist: CareerBlacklistEntry[];
  stories: CareerStory[];
  busy: boolean;
  saving: boolean;
  savedFlash: boolean;
  onSave: () => void;
  onFillFromCV: () => void;
  onReload: () => Promise<void>;
  onError: (msg: string) => void;
}) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [storyTitle, setStoryTitle] = useState("");
  const [storySit, setStorySit] = useState("");

  async function onUpload(file: File | undefined) {
    if (!file) return;
    setUploading(true);
    try {
      if (!file.name.toLowerCase().endsWith(".pdf") && file.type !== "application/pdf") {
        toast.error("Upload a PDF.");
        return;
      }
      const prop = await uploadCareerCV(file);
      if (prop.patch.cv_markdown) setCvDraft(prop.patch.cv_markdown);
      if (prop.patch.identity?.full_name) setFullName(prop.patch.identity.full_name);
      await onReload();
      toast.success("CV PDF saved.");
    } catch (e: unknown) {
      const msg = apiErrorMessage(e, "Could not read that file.");
      onError(msg);
      toast.error(msg);
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        Upload a PDF CV. We save it and use the extracted text to score jobs and tailor a new PDF.
      </p>
      <div>
        <FieldLabel
          htmlFor="career-name"
          label="Name"
          hint="Your name as it should appear on cover letters and email drafts."
        />
        <input
          id="career-name"
          value={fullName}
          onChange={(e) => setFullName(e.target.value)}
          className="w-full max-w-md rounded-md border border-input bg-background px-3 py-2 text-sm"
        />
      </div>
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={sponsorship}
          onChange={(e) => setSponsorship(e.target.checked)}
        />
        I need visa sponsorship
        <FieldHint text="Check this if you need a visa. Postings that say no sponsorship become a hard stop, not a lower score." />
      </label>
      <div>
        <FieldLabel
          htmlFor="career-titles"
          label="Target titles (comma-separated)"
          hint="Roles you want, e.g. Head of AI, Staff engineer. Used to filter portal scans and judge fit."
        />
        <input
          id="career-titles"
          value={titles}
          onChange={(e) => setTitles(e.target.value)}
          className="w-full max-w-md rounded-md border border-input bg-background px-3 py-2 text-sm"
          placeholder="Head of AI, Staff engineer"
        />
      </div>
      <div>
        <FieldLabel
          htmlFor="career-comp"
          label="Target compensation"
          hint="Minimum or range you will consider, e.g. £180k+. Compared to the posting in Block D."
        />
        <input
          id="career-comp"
          value={minComp}
          onChange={(e) => setMinComp(e.target.value)}
          className="w-full max-w-md rounded-md border border-input bg-background px-3 py-2 text-sm"
          placeholder="£180k+"
        />
      </div>
      <div>
        <FieldLabel
          htmlFor="career-house-rules"
          label="House rules"
          hint="Your scoring taste, not your CV. Example: skip agency roles, remote-UK only. Can make scoring stricter, never looser than 4.0 apply / 4.5 form answers. Leave blank if unsure."
        />
        <textarea
          id="career-house-rules"
          value={houseRules}
          onChange={(e) => setHouseRules(e.target.value)}
          rows={3}
          className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
          placeholder="Scoring overrides. Floors cannot drop below 4.0 / 4.5."
        />
      </div>
      <div>
        <FieldLabel
          htmlFor="career-cv-upload"
          label="CV"
          hint="PDF only. Upload saves it immediately. Get tailored CV downloads a new PDF for that job."
        />
        <div className="flex flex-wrap items-center gap-2">
          <input
            id="career-cv-upload"
            ref={fileRef}
            type="file"
            accept=".pdf,application/pdf"
            className="sr-only"
            onChange={(e) => void onUpload(e.target.files?.[0])}
          />
          <button
            type="button"
            className="rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
            disabled={busy || uploading}
            onClick={() => fileRef.current?.click()}
          >
            {uploading ? "Saving PDF…" : cvDraft.trim() ? "Replace CV PDF" : "Upload CV"}
          </button>
          <span className="text-xs text-muted-foreground">
            {cvDraft.trim() ? "PDF saved. Upload another to replace it." : "PDF only, up to 5MB."}
          </span>
        </div>
        {cvDraft.trim() && (
          <textarea
            id="career-cv"
            value={cvDraft}
            readOnly
            rows={10}
            className="mt-2 w-full rounded-md border border-input bg-muted/40 px-3 py-2 font-mono text-sm"
            aria-label="Extracted CV text"
          />
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={onSave}
          disabled={busy}
          className="rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
        >
          {saving ? "Saving…" : savedFlash ? "Saved" : "Save profile"}
        </button>
        <button
          type="button"
          onClick={onFillFromCV}
          disabled={busy || !cvDraft.trim()}
          className="rounded-md border border-border px-3 py-2 text-sm disabled:opacity-50"
        >
          Fill name from CV
        </button>
        {savedFlash && <span className="text-sm text-muted-foreground">Profile saved.</span>}
      </div>
      <div className="border-t border-border pt-4">
        <p className="mb-2 flex items-center gap-1.5 text-sm font-medium">
          Blacklist
          <FieldHint text="Companies you never want recommended. Evaluate will ask before continuing — it never silently skips." />
        </p>
        <ul className="mb-2 text-sm text-muted-foreground">
          {blacklist.length === 0 ? (
            <li>Empty — evaluate will never skip a company silently.</li>
          ) : (
            blacklist.map((b) => (
              <li key={b.id}>
                {b.company || b.domain}
                {b.reason ? ` — ${b.reason}` : ""}
              </li>
            ))
          )}
        </ul>
        <BlacklistForm
          onAdd={async (company, reason) => {
            await addCareerBlacklist({ company, reason });
            await onReload();
          }}
        />
      </div>
      <div className="border-t border-border pt-4">
        <p className="mb-2 flex items-center gap-1.5 text-sm font-medium">
          Story bank (STAR+R)
          <FieldHint text="Interview stories: title plus situation. Used for interview prep." />
        </p>
        {stories.length === 0 ? (
          <p className="mb-2 text-sm text-muted-foreground">None yet. Optional for a first pass.</p>
        ) : (
          <ul className="mb-2 space-y-1 text-sm text-muted-foreground">
            {stories.map((s) => (
              <li key={s.id}>
                {s.title} <span className="text-xs">({s.provenance})</span>
              </li>
            ))}
          </ul>
        )}
        <form
          className="flex flex-wrap gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (!storyTitle.trim()) return;
            void upsertCareerStory({
              title: storyTitle,
              situation: storySit,
              provenance: "user_stated",
            })
              .then(() => {
                setStoryTitle("");
                setStorySit("");
                return onReload();
              })
              .catch((err) => onError(apiErrorMessage(err, "Could not save story.")));
          }}
        >
          <input
            value={storyTitle}
            onChange={(e) => setStoryTitle(e.target.value)}
            placeholder="Story title"
            className="rounded-md border border-input bg-background px-2 py-1 text-sm"
          />
          <input
            value={storySit}
            onChange={(e) => setStorySit(e.target.value)}
            placeholder="Situation"
            className="min-w-[12rem] flex-1 rounded-md border border-input bg-background px-2 py-1 text-sm"
          />
          <button type="submit" className="rounded-md border border-border px-2 py-1 text-sm">
            Add story
          </button>
        </form>
      </div>
    </div>
  );
}

function BlacklistForm({
  onAdd,
}: {
  onAdd: (company: string, reason: string) => Promise<void>;
}) {
  const [company, setCompany] = useState("");
  const [reason, setReason] = useState("");
  return (
    <form
      className="flex flex-wrap gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        void onAdd(company, reason).then(() => {
          setCompany("");
          setReason("");
        });
      }}
    >
      <input
        value={company}
        onChange={(e) => setCompany(e.target.value)}
        placeholder="Company"
        className="rounded-md border border-input bg-background px-2 py-1 text-sm"
      />
      <input
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        placeholder="Reason"
        className="rounded-md border border-input bg-background px-2 py-1 text-sm"
      />
      <button type="submit" className="rounded-md border border-border px-2 py-1 text-sm">
        Add
      </button>
    </form>
  );
}
