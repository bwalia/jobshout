import { apiClient } from "@/lib/api/client";
import type {
  CareerApplication,
  CareerArtifact,
  CareerBlacklistEntry,
  CareerDoctorReport,
  CareerEvaluateResult,
  CareerEvaluation,
  CareerFollowup,
  CareerIntakeProposal,
  CareerInterviewPrep,
  CareerOfferPrep,
  CareerPatterns,
  CareerPipelineItem,
  CareerPortal,
  CareerProfile,
  CareerSalaryGap,
  CareerStory,
  Paginated,
} from "@/types/career";

export async function getCareerProfile() {
  const { data } = await apiClient.get<CareerProfile>("/career/profile");
  return data;
}

export async function patchCareerProfile(payload: Record<string, unknown>) {
  const { data } = await apiClient.patch<CareerProfile>("/career/profile", payload);
  return data;
}

export async function careerIntake(document: string) {
  const { data } = await apiClient.post<CareerIntakeProposal>("/career/intake", {
    document,
  });
  return data;
}

export async function evaluateCareer(payload: {
  job_url?: string;
  jd_text?: string;
  mode?: string;
  tailor_cv?: boolean;
  confirm_blacklist?: boolean;
}) {
  const { data } = await apiClient.post<CareerEvaluateResult>("/career/evaluate", payload, {
    timeout: 180_000,
  });
  return data;
}

export async function listCareerEvaluations() {
  const { data } = await apiClient.get<Paginated<CareerEvaluation>>("/career/evaluations", {
    params: { per_page: 50 },
  });
  return data;
}

export async function getCareerEvaluation(id: string) {
  const { data } = await apiClient.get<CareerEvaluation>(`/career/evaluations/${id}`);
  return data;
}

export async function listCareerPipeline() {
  const { data } = await apiClient.get<Paginated<CareerPipelineItem>>("/career/pipeline", {
    params: { per_page: 50 },
  });
  return data;
}

export async function listCareerTracker(status?: string) {
  const { data } = await apiClient.get<Paginated<CareerApplication>>("/career/applications", {
    params: { per_page: 50, status: status || undefined },
  });
  return data;
}

export async function setCareerStatus(id: string, status: string, note?: string) {
  const { data } = await apiClient.post<CareerApplication>(
    `/career/applications/${id}/status`,
    { status, note }
  );
  return data;
}

export async function careerDoctor() {
  const { data } = await apiClient.get<CareerDoctorReport>("/career/doctor");
  return data;
}

export async function careerPatterns() {
  const { data } = await apiClient.get<CareerPatterns>("/career/patterns");
  return data;
}

export async function careerScan(payload: {
  board?: string;
  slug?: string;
  company?: string;
  query?: string;
}) {
  const { data } = await apiClient.post("/career/scan", payload, { timeout: 120_000 });
  return data;
}

export async function listCareerPortals() {
  const { data } = await apiClient.get<CareerPortal[]>("/career/portals");
  return data;
}

export async function addCareerPortal(payload: {
  board: string;
  slug: string;
  company?: string;
}) {
  const { data } = await apiClient.post<CareerPortal>("/career/portals", payload);
  return data;
}

export async function listCareerBlacklist() {
  const { data } = await apiClient.get<CareerBlacklistEntry[]>("/career/blacklist");
  return data;
}

export async function addCareerBlacklist(payload: {
  company?: string;
  domain?: string;
  reason?: string;
}) {
  const { data } = await apiClient.post<CareerBlacklistEntry>("/career/blacklist", payload);
  return data;
}

export async function tailorCareerCV(evaluationId: string) {
  const { data } = await apiClient.post<CareerArtifact>(`/career/evaluations/${evaluationId}/cv`, {}, {
    timeout: 120_000,
  });
  return data;
}

export async function careerCoverLetter(evaluationId: string) {
  const { data } = await apiClient.post<CareerArtifact>(
    `/career/evaluations/${evaluationId}/cover`,
    {},
    { timeout: 120_000 }
  );
  return data;
}

export async function careerEmailDraft(evaluationId: string) {
  const { data } = await apiClient.post<CareerArtifact>(
    `/career/evaluations/${evaluationId}/email`,
    {},
    { timeout: 120_000 }
  );
  return data;
}

export async function listCareerArtifacts(applicationId: string) {
  const { data } = await apiClient.get<CareerArtifact[]>(
    `/career/applications/${applicationId}/artifacts`
  );
  return data;
}

export async function listCareerStories() {
  const { data } = await apiClient.get<CareerStory[]>("/career/stories");
  return data;
}

export async function upsertCareerStory(payload: Partial<CareerStory> & { title: string }) {
  const { data } = await apiClient.post<CareerStory>("/career/stories", payload);
  return data;
}

export async function listCareerFollowups() {
  const { data } = await apiClient.get<CareerFollowup[]>("/career/followups");
  return data;
}

export async function careerFollowup(applicationId: string) {
  const { data } = await apiClient.post<CareerFollowup>(
    `/career/applications/${applicationId}/followup`
  );
  return data;
}

export async function careerInterviewPrep(applicationId: string) {
  const { data } = await apiClient.post<CareerInterviewPrep>(
    `/career/applications/${applicationId}/interview-prep`
  );
  return data;
}

export async function careerOfferPrep(applicationId: string) {
  const { data } = await apiClient.post<CareerOfferPrep>(
    `/career/applications/${applicationId}/offer-prep`
  );
  return data;
}

export async function careerSalaryGap(
  applicationId: string,
  advertised?: string,
  actual?: string
) {
  const { data } = await apiClient.post<CareerSalaryGap>(
    `/career/applications/${applicationId}/salary-gap`,
    { advertised, actual }
  );
  return data;
}

export async function careerBatchEvaluate() {
  const { data } = await apiClient.post("/career/pipeline/batch", { limit: 8 }, { timeout: 180_000 });
  return data;
}
