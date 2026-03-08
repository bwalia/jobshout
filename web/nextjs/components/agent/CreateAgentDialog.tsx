"use client";

import { useEffect, useRef } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { X } from "lucide-react";
import { useCreateAgent } from "@/lib/hooks/useAgents";
import type { CreateAgentRequest } from "@/lib/types/agent";

// ---------------------------------------------------------------------------
// Validation schema
// ---------------------------------------------------------------------------
const createAgentSchema = z.object({
  name: z.string().min(1, "Name is required").max(100, "Name is too long"),
  role: z.string().min(1, "Role is required").max(100, "Role is too long"),
  description: z.string().max(500, "Description is too long").optional(),
  model_provider: z.string().max(50).optional(),
  model_name: z.string().max(100).optional(),
  system_prompt: z.string().max(10000, "System prompt is too long").optional(),
});

type CreateAgentFormValues = z.infer<typeof createAgentSchema>;

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------
interface CreateAgentDialogProps {
  open: boolean;
  onClose: () => void;
}

// ---------------------------------------------------------------------------
// Shared form field components
// ---------------------------------------------------------------------------
function FieldLabel({
  htmlFor,
  children,
  required,
}: {
  htmlFor: string;
  children: React.ReactNode;
  required?: boolean;
}) {
  return (
    <label
      htmlFor={htmlFor}
      className="block text-sm font-medium text-foreground"
    >
      {children}
      {required && (
        <span className="ml-1 text-destructive" aria-hidden="true">
          *
        </span>
      )}
    </label>
  );
}

function FieldError({ message }: { message?: string }) {
  if (!message) return null;
  return <p className="mt-1 text-xs text-destructive">{message}</p>;
}

// ---------------------------------------------------------------------------
// Dialog component
// ---------------------------------------------------------------------------
export function CreateAgentDialog({ open, onClose }: CreateAgentDialogProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const { mutate: createAgent, isPending } = useCreateAgent();

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<CreateAgentFormValues>({
    resolver: zodResolver(createAgentSchema),
    defaultValues: {
      name: "",
      role: "",
      description: "",
      model_provider: "",
      model_name: "",
      system_prompt: "",
    },
  });

  // Reset the form whenever the dialog is opened fresh
  useEffect(() => {
    if (open) {
      reset();
    }
  }, [open, reset]);

  // Close on Escape key
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        onClose();
      }
    }
    if (open) {
      document.addEventListener("keydown", handleKeyDown);
    }
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [open, onClose]);

  // Prevent scroll on the body while the dialog is open
  useEffect(() => {
    if (open) {
      document.body.style.overflow = "hidden";
    }
    return () => {
      document.body.style.overflow = "";
    };
  }, [open]);

  function onSubmit(values: CreateAgentFormValues) {
    // Build the payload, omitting empty optional strings
    const payload: CreateAgentRequest = {
      name: values.name,
      role: values.role,
      ...(values.description?.trim() && {
        description: values.description.trim(),
      }),
      ...(values.model_provider?.trim() && {
        model_provider: values.model_provider.trim(),
      }),
      ...(values.model_name?.trim() && {
        model_name: values.model_name.trim(),
      }),
      ...(values.system_prompt?.trim() && {
        system_prompt: values.system_prompt.trim(),
      }),
    };

    createAgent(payload, {
      onSuccess: () => {
        onClose();
      },
    });
  }

  if (!open) return null;

  return (
    /* Backdrop */
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="create-agent-title"
    >
      {/* Overlay */}
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Dialog panel */}
      <div
        ref={dialogRef}
        className="relative z-10 w-full max-w-lg rounded-lg border border-border bg-card shadow-xl"
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border px-6 py-4">
          <h2
            id="create-agent-title"
            className="text-base font-semibold text-foreground"
          >
            Create New Agent
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-sm text-muted-foreground hover:text-foreground transition-colors"
            aria-label="Close dialog"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Form body */}
        <form
          onSubmit={handleSubmit(onSubmit)}
          className="flex flex-col gap-5 overflow-y-auto px-6 py-5"
          style={{ maxHeight: "calc(100vh - 12rem)" }}
        >
          {/* Name */}
          <div>
            <FieldLabel htmlFor="agent-name" required>
              Name
            </FieldLabel>
            <input
              id="agent-name"
              type="text"
              placeholder="e.g. Content Writer"
              {...register("name")}
              className="mt-1.5 flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            <FieldError message={errors.name?.message} />
          </div>

          {/* Role */}
          <div>
            <FieldLabel htmlFor="agent-role" required>
              Role
            </FieldLabel>
            <input
              id="agent-role"
              type="text"
              placeholder="e.g. Marketing Copywriter"
              {...register("role")}
              className="mt-1.5 flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            <FieldError message={errors.role?.message} />
          </div>

          {/* Description */}
          <div>
            <FieldLabel htmlFor="agent-description">Description</FieldLabel>
            <textarea
              id="agent-description"
              rows={3}
              placeholder="Brief description of what this agent does…"
              {...register("description")}
              className="mt-1.5 flex w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            <FieldError message={errors.description?.message} />
          </div>

          {/* Model Provider + Model Name (side by side on wider screens) */}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <FieldLabel htmlFor="agent-model-provider">
                Model Provider
              </FieldLabel>
              <input
                id="agent-model-provider"
                type="text"
                placeholder="e.g. openai, anthropic"
                {...register("model_provider")}
                className="mt-1.5 flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
              <FieldError message={errors.model_provider?.message} />
            </div>

            <div>
              <FieldLabel htmlFor="agent-model-name">Model Name</FieldLabel>
              <input
                id="agent-model-name"
                type="text"
                placeholder="e.g. gpt-4o, claude-3-5"
                {...register("model_name")}
                className="mt-1.5 flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
              <FieldError message={errors.model_name?.message} />
            </div>
          </div>

          {/* System Prompt */}
          <div>
            <FieldLabel htmlFor="agent-system-prompt">System Prompt</FieldLabel>
            <p className="mt-0.5 text-xs text-muted-foreground">
              Instructions that define the agent's behaviour and constraints.
            </p>
            <textarea
              id="agent-system-prompt"
              rows={5}
              placeholder="You are a helpful assistant that…"
              {...register("system_prompt")}
              className="mt-1.5 flex w-full resize-y rounded-md border border-input bg-background px-3 py-2 font-mono text-xs ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            <FieldError message={errors.system_prompt?.message} />
          </div>

          {/* Footer actions */}
          <div className="flex items-center justify-end gap-3 border-t border-border pt-4">
            <button
              type="button"
              onClick={onClose}
              disabled={isPending}
              className="inline-flex h-9 items-center rounded-md border border-input bg-background px-4 text-sm font-medium hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isPending}
              className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50 transition-colors"
            >
              {isPending ? "Creating…" : "Create Agent"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
