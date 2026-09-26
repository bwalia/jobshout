"use client";

import type { ComponentType } from "react";
import { AbTestAgentClient } from "@/components/AbTestAgentClient";
import { CareerAgentClient } from "@/components/CareerAgentClient";
import { CreditControllerAgentClient } from "@/components/CreditControllerAgentClient";
import { MailAgentClient } from "@/components/MailAgentClient";
import { PentestAgentClient } from "@/components/PentestAgentClient";
import { ReviewAgentClient } from "@/components/ReviewAgentClient";
import { SimproPaymentsAgentClient } from "@/components/SimproPaymentsAgentClient";
import { WafLabAgentClient } from "@/components/WafLabAgentClient";
import { SeoAgentClient } from "@/components/SeoAgentClient";
import { SecretsRotationAgentClient } from "@/components/SecretsRotationAgentClient";
import { LinuxPatchAgentClient } from "@/components/LinuxPatchAgentClient";
import { ArticlesView } from "@/components/articles/ArticlesView";
import { ImagesView } from "@/components/image/ImagesView";
import { CourseGeneratorClient } from "@/components/courses/CourseGeneratorClient";
import { InsightsWriterClient } from "@/components/insights-writer/InsightsWriterClient";

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
  credit_controller: CreditControllerAgentClient,
  simpro_payments: SimproPaymentsAgentClient,
  waf_lab: WafLabAgentClient,
  seo_analyst: SeoAgentClient,
  secrets_rotation: SecretsRotationAgentClient,
  linux_patch: LinuxPatchAgentClient,
  ab_testing: AbTestAgentClient,
  article_writer: function ArticleWriterClient() {
    return <ArticlesView hideHeader />;
  },
  images: function ImageGeneratorClient() {
    return <ImagesView hideHeader />;
  },
  course_generator: CourseGeneratorClient,
  jobshout_com_writer: InsightsWriterClient,
};
