import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  generateImage,
  getGeneratedImages,
  getImageModels,
} from "@/lib/api/images";
import type { GenerateImageRequest } from "@/lib/types/image";

export const imageKeys = {
  all: ["images"] as const,
  models: () => [...imageKeys.all, "models"] as const,
  list: () => [...imageKeys.all, "list"] as const,
};

/**
 * What this platform can draw with. Kept fresh for five minutes: the server
 * already caches discovery, and a model appearing on the workstation is rare.
 */
export function useImageModels() {
  return useQuery({
    queryKey: imageKeys.models(),
    queryFn: getImageModels,
    staleTime: 5 * 60 * 1000,
  });
}

export function useGeneratedImages(limit = 50) {
  return useQuery({
    queryKey: [...imageKeys.list(), limit],
    queryFn: () => getGeneratedImages(limit),
  });
}

export function useGenerateImage() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (req: GenerateImageRequest) => generateImage(req),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: imageKeys.list() });
      // The duration is worth showing rather than hiding: it explains a wait
      // the user just sat through, and it is how they learn that a turbo model
      // is the difference between 25 seconds and several minutes.
      toast.success(`Image generated in ${(result.duration_ms / 1000).toFixed(1)}s`);
    },
    onError: (error: unknown) => {
      const message =
        (error as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        (error as Error)?.message ??
        "Image generation failed";
      toast.error(message);
    },
  });
}
