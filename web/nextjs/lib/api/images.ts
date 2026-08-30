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
 * Fetch a stored image and hand back an object URL the browser can render.
 *
 * A stored image is served from `/api/v1/images/file/…`, which sits inside the
 * authenticated part of the API — and an `<img src>` sends no Authorization
 * header, so pointing one straight at that path produces a broken image every
 * time. Going through the API client instead means the token is attached and a
 * 401 still refreshes, at the cost of holding the bytes in memory until the
 * caller revokes the URL.
 *
 * The stored URL already carries the `/api/v1` prefix that the client adds, so
 * it is stripped here rather than sent twice.
 */
export async function fetchImageObjectURL(url: string): Promise<string> {
  const { data } = await apiClient.get<Blob>(url.replace(/^\/api\/v1/, ""), {
    responseType: "blob",
  });
  return URL.createObjectURL(data);
}

export async function getGeneratedImages(limit = 50): Promise<GeneratedImage[]> {
  const { data } = await apiClient.get<{ images: GeneratedImage[] }>("/images", {
    params: { limit },
  });
  return data.images ?? [];
}
