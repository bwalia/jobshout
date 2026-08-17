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

export async function getGeneratedImages(limit = 50): Promise<GeneratedImage[]> {
  const { data } = await apiClient.get<{ images: GeneratedImage[] }>("/images", {
    params: { limit },
  });
  return data.images ?? [];
}
