"use client";

import type { ComponentType } from "react";
import { CareerAgentClient } from "@/components/CareerAgentClient";
import { MailAgentClient } from "@/components/MailAgentClient";
import { PentestAgentClient } from "@/components/PentestAgentClient";
import { ReviewAgentClient } from "@/components/ReviewAgentClient";
import { ArticlesView } from "@/components/articles/ArticlesView";
import { ImagesView } from "@/components/image/ImagesView";

/**
 * Optional product UI that owns the Task Manager tab (replaces the schema form).
 * Schema fields stay on New task / Run task / chat.
 *
 * Keyed by metadata.builtin. Register here; do not add a TaskManagerPanel branch.
 */
export const AGENT_CLIENTS: Record<string, ComponentType> = {
  career_ops: CareerAgentClient,
  mail: MailAgentClient,
  pentester: PentestAgentClient,
  pr_reviewer: ReviewAgentClient,
  article_writer: function ArticleWriterClient() {
    return <ArticlesView hideHeader />;
  },
  images: function ImageGeneratorClient() {
    return <ImagesView hideHeader />;
  },
};
