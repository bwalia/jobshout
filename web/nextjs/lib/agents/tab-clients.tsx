"use client";

import type { ComponentType } from "react";
import { CareerAgentClient } from "@/components/CareerAgentClient";
import { MailAgentClient } from "@/components/MailAgentClient";
import { PentestAgentClient } from "@/components/PentestAgentClient";
import { ReviewAgentClient } from "@/components/ReviewAgentClient";
import { ArticlesView } from "@/components/articles/ArticlesView";
import { ImagesView } from "@/components/image/ImagesView";

/**
 * Optional product UI under the generic Task Manager tab form.
 *
 * All specialists are wired this way: keyed by metadata.builtin.
 * A new agent does not need a TaskManagerPanel if — register a client here,
 * do not add a switch.
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
