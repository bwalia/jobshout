export type Role = "recruiter" | "candidate";

export type SessionState = {
  id: string;
  role: Role | null;
  docId: string | null;
  contactSaved: boolean;
  brief: string;
  cv: string;
  name: string;
  email: string;
  phone: string;
  company: string;
  /** When set, chat is scoped to applying for this board job. */
  jobId: string | null;
  jobTitle: string;
  jobSummary: string;
  jobRequirements: string[];
  /** True after a successful applications API submit for jobId. */
  applied: boolean;
};
