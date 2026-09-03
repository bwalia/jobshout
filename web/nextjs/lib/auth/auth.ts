import { API_BASE_URL, apiClient } from "@/lib/api/client";

export interface AuthUser {
  id: string;
  email: string;
  full_name: string;
  avatar_url: string | null;
  role: string;
  org_id: string | null;
  created_at: string;
  updated_at: string;
}

export interface AuthResponse {
  access_token: string;
  refresh_token: string;
  user: AuthUser;
}

export interface LoginPayload {
  email: string;
  password: string;
}

export interface RegisterPayload {
  email: string;
  password: string;
  full_name: string;
  org_name: string;
}

const ACCESS_TOKEN_KEY = "access_token";
const REFRESH_TOKEN_KEY = "refresh_token";

export function getAccessToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function getRefreshToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(REFRESH_TOKEN_KEY);
}

export function setTokens(accessToken: string, refreshToken: string): void {
  localStorage.setItem(ACCESS_TOKEN_KEY, accessToken);
  localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken);
}

export function clearTokens(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
}

export async function loginUser(payload: LoginPayload): Promise<AuthResponse> {
  const { data } = await apiClient.post<AuthResponse>(
    "/auth/login",
    payload
  );
  setTokens(data.access_token, data.refresh_token);
  return data;
}

export async function registerUser(
  payload: RegisterPayload
): Promise<AuthResponse> {
  const { data } = await apiClient.post<AuthResponse>(
    "/auth/register",
    payload
  );
  setTokens(data.access_token, data.refresh_token);
  return data;
}

export async function refreshAccessToken(): Promise<string | null> {
  const refreshToken = getRefreshToken();
  if (!refreshToken) return null;

  try {
    const { data } = await apiClient.post<AuthResponse>("/auth/refresh", {
      refresh_token: refreshToken,
    });
    setTokens(data.access_token, data.refresh_token);
    return data.access_token;
  } catch {
    clearTokens();
    return null;
  }
}

export async function fetchCurrentUser(): Promise<AuthUser | null> {
  try {
    const { data } = await apiClient.get<AuthUser>("/auth/me");
    return data;
  } catch {
    return null;
  }
}

export function googleAuthStartHref(opts?: {
  intent?: "login" | "signup";
  orgName?: string;
}): string {
  const params = new URLSearchParams();
  params.set("intent", opts?.intent ?? "login");
  const orgName = opts?.orgName?.trim();
  if (orgName) params.set("org_name", orgName);
  const qs = params.toString();
  const path = `/api/v1/auth/google/start?${qs}`;

  // Cluster nginx serves UI and API on one host at /api/v1. Helm's
  // publicApiURL is often `https://<host>/api`, and concatenating /api/v1
  // onto that would 404. Same-origin /api/v1 is the public callback path.
  // Locally the UI (:3001) and API (:8190) differ, so use the API origin.
  if (typeof window !== "undefined") {
    const env = API_BASE_URL.replace(/\/$/, "");
    try {
      const api = new URL(env);
      if (api.origin !== window.location.origin) {
        return `${env}${path}`;
      }
    } catch {
      // fall through to same-origin
    }
    return `${window.location.origin}${path}`;
  }
  return path;
}

export async function fetchGoogleAuthEnabled(): Promise<boolean> {
  try {
    const { data } = await apiClient.get<{ enabled: boolean }>("/auth/google/status");
    return Boolean(data.enabled);
  } catch {
    return false;
  }
}

export async function completeGoogleAuth(ticket: string): Promise<AuthResponse> {
  const { data } = await apiClient.post<AuthResponse>("/auth/google/complete", {
    ticket,
  });
  setTokens(data.access_token, data.refresh_token);
  return data;
}

export function googleAuthErrorMessage(code: string | null): string | null {
  if (!code) return null;
  switch (code) {
    case "denied":
      return "Google sign-in was cancelled.";
    case "not_configured":
      return "Google sign-in is not configured on this server.";
    case "invalid_state":
      return "Google sign-in expired. Try again.";
    case "unverified_email":
      return "That Google account's email is not verified.";
    case "missing_code":
      return "Google sign-in did not complete. Try again.";
    default:
      return "Google sign-in failed. Try again.";
  }
}
