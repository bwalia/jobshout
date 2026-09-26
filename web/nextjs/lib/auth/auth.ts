import axios from "axios";
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

export class GoogleAuthCompleteError extends Error {
  constructor(readonly code: "expired" | "failed") {
    super(code === "expired" ? "Google sign-in expired. Try again." : "Google sign-in failed. Try again.");
    this.name = "GoogleAuthCompleteError";
  }
}

/** API v1 root for Google OAuth browser calls (status, start, complete). */
export function authApiV1Base(): string {
  const env = API_BASE_URL.replace(/\/$/, "");
  if (typeof window !== "undefined") {
    try {
      if (env) {
        const api = new URL(env);
        if (api.origin !== window.location.origin) {
          return `${env}/api/v1`;
        }
      }
    } catch {
      // Empty NEXT_PUBLIC_API_URL in cluster image builds.
    }
    return `${window.location.origin}/api/v1`;
  }
  return env ? `${env}/api/v1` : "/api/v1";
}

export function googleAuthStartHref(opts?: {
  intent?: "login" | "signup";
  orgName?: string;
}): string {
  const params = new URLSearchParams();
  params.set("intent", opts?.intent ?? "login");
  const orgName = opts?.orgName?.trim();
  if (orgName) params.set("org_name", orgName);
  return `${authApiV1Base()}/auth/google/start?${params.toString()}`;
}

export async function fetchGoogleAuthEnabled(): Promise<boolean> {
  try {
    // Plain axios: a stale Bearer on apiClient must not 401-refresh this public probe.
    const { data } = await axios.get<{ enabled: boolean }>(
      `${authApiV1Base()}/auth/google/status`
    );
    return Boolean(data.enabled);
  } catch {
    return false;
  }
}

export async function completeGoogleAuth(ticket: string): Promise<AuthResponse> {
  try {
    // Do not use apiClient: its 401 interceptor redirects to /login and
    // swallows an expired ticket before the callback page can explain it.
    const { data } = await axios.post<AuthResponse>(
      `${authApiV1Base()}/auth/google/complete`,
      { ticket }
    );
    setTokens(data.access_token, data.refresh_token);
    return data;
  } catch (err) {
    if (axios.isAxiosError(err) && err.response?.status === 401) {
      throw new GoogleAuthCompleteError("expired");
    }
    throw new GoogleAuthCompleteError("failed");
  }
}
