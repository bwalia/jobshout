/** One image model a provider can run. Mirrors imagegen.ModelInfo exactly. */
export interface ImageModel {
  name: string;
  provider: string;
  /** Upstream weights, where the provider has such a notion. */
  repo?: string;
  /**
   * Whether this model can generate now. A local model whose weights are not
   * downloaded is known but not available — selecting it starts a
   * multi-gigabyte download instead of drawing a picture, so the picker says so
   * rather than hiding the difference.
   */
  available: boolean;
  /** Reaches a usable image in a handful of steps. */
  fast: boolean;
}

/** GET /images/models. */
export interface ImageModelsResponse {
  /** False when no provider is configured. The UI renders a disabled control. */
  enabled: boolean;
  default_provider: string;
  models: ImageModel[];
}

/** POST /images/generate. */
export interface GenerateImageRequest {
  prompt: string;
  provider?: string;
  model?: string;
  width?: number;
  height?: number;
  steps?: number;
  /** Reuse a previous result's seed to reproduce that exact image. */
  seed?: number;
}

export interface GenerateImageResponse {
  /** Empty when object storage is not configured; image_base64 is set instead. */
  url?: string;
  image_base64?: string;
  provider: string;
  model: string;
  seed: number;
  width: number;
  height: number;
  duration_ms: number;
  id?: string;
}

/** One row of the org's generated-image history. */
export interface GeneratedImage {
  id: string;
  org_id: string;
  created_by?: string;
  prompt: string;
  provider: string;
  model: string;
  seed: number;
  width: number;
  height: number;
  url?: string;
  /** What asked for it: blog_cover, blog_inline, agent_tool or manual. */
  source: string;
  duration_ms: number;
  created_at: string;
}
