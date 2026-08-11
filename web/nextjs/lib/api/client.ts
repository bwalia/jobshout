import axios, { type AxiosError, type InternalAxiosRequestConfig } from "axios";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export const apiClient = axios.create({
  baseURL: `${API_BASE_URL}/api/v1`,
  headers: {
    "Content-Type": "application/json",
  },
  timeout: 30000,
});

// Track in-flight refresh to prevent concurrent refresh requests
let refreshPromise: Promise<string> | null = null;

interface ExtendedAxiosRequestConfig extends InternalAxiosRequestConfig {
  _retry?: boolean;
}

// Request interceptor: attach access token from localStorage
apiClient.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    if (typeof window !== "undefined") {
      const token = localStorage.getItem("access_token");
      if (token) {
        config.headers.Authorization = `Bearer ${token}`;
      }
    }
    return config;
  },
  (error: AxiosError) => Promise.reject(error)
);

// Response interceptor: handle 401 with token refresh
apiClient.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const originalRequest = error.config as ExtendedAxiosRequestConfig | undefined;

    if (
      error.response?.status === 401 &&
      originalRequest &&
      !originalRequest._retry
    ) {
      originalRequest._retry = true;

      try {
        if (!refreshPromise) {
          refreshPromise = refreshAccessToken();
        }

        const newToken = await refreshPromise;
        refreshPromise = null;

        localStorage.setItem("access_token", newToken);
        originalRequest.headers.Authorization = `Bearer ${newToken}`;
        return apiClient(originalRequest);
      } catch (refreshError) {
        refreshPromise = null;
        localStorage.removeItem("access_token");
        localStorage.removeItem("refresh_token");

        if (typeof window !== "undefined") {
          window.location.href = "/login";
        }
        return Promise.reject(refreshError);
      }
    }

    return Promise.reject(error);
  }
);

async function refreshAccessToken(): Promise<string> {
  const refreshToken = localStorage.getItem("refresh_token");
  if (!refreshToken) {
    throw new Error("No refresh token available");
  }

  const response = await axios.post(
    `${API_BASE_URL}/api/v1/auth/refresh`,
    { refresh_token: refreshToken }
  );

  const { access_token, refresh_token: newRefreshToken } = response.data;
  localStorage.setItem("refresh_token", newRefreshToken);
  return access_token;
}

export interface APIError {
  error: string;
  message?: string;
}

/**
 * Pull the server's own explanation out of a failed request.
 *
 * Axios reports "Request failed with status code 400", which tells a user
 * nothing. The API answers with `{"error": "..."}` carrying the actual reason
 * — "publishing is not configured", "run has already been published" — and
 * that is what belongs in the toast. Falls back when the shape is unexpected.
 */
export function apiErrorMessage(err: unknown, fallback: string): string {
  if (axios.isAxiosError(err)) {
    const data = err.response?.data as APIError | undefined;
    if (data?.error) return data.error;
    if (data?.message) return data.message;
  }
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}
