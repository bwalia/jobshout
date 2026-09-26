import { apiClient } from "@/lib/api/client";
import type {
  GeneratedImage,
  GenerateImageRequest,
  GenerateImageResponse,
  ImageModelsResponse,
} from "@/lib/types/image";

export async function getImageModels(): Promise<ImageModelsResponse> {
  const { data } = await apiClient.get<ImageModelsResponse>("/images/models");
  return data;
}

/**
 * Generation is slow by design — a cold model load plus the denoising steps is
 * tens of seconds — so this call is given its own long timeout rather than the
 * client default, which would abort a request that was working perfectly well.
 */
export async function generateImage(
  req: GenerateImageRequest,
): Promise<GenerateImageResponse> {
  const { data } = await apiClient.post<GenerateImageResponse>("/images/generate", req, {
    timeout: 10 * 60 * 1000,
  });
  return data;
}

/**
 * Turn a stored image path into a URL the browser can put on `<img src>`.
 *
 * Generated files are public (`GET /api/v1/images/file/…`, UUID keys). A
 * relative path would hit this Next.js origin and 404; a cross-origin axios
 * fetch needs CORS to match the exact UI host (`localhost` vs `127.0.0.1`).
 * Pointing at the API origin works from either, with no token and no blob URL.
 */
export function resolveStoredImageSrc(src: string): string {
  const trimmed = src.trim();
  if (!trimmed || trimmed.startsWith("data:") || /^https?:\/\//i.test(trimmed)) {
    return trimmed;
  }
  const base = (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080").replace(
    /\/$/,
    ""
  );
  return trimmed.startsWith("/") ? `${base}${trimmed}` : `${base}/${trimmed}`;
}

export async function getGeneratedImages(limit = 50): Promise<GeneratedImage[]> {
  const { data } = await apiClient.get<{ images: GeneratedImage[] }>("/images", {
    params: { limit },
  });
  return data.images ?? [];
}
