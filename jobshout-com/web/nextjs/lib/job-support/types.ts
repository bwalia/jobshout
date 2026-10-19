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
};
