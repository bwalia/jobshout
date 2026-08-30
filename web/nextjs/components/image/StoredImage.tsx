"use client";

import { resolveStoredImageSrc } from "@/lib/api/images";

/**
 * Renders a stored image URL in the JobShout UI.
 *
 * Generated images are public at `/api/v1/images/file/…`. The path is resolved
 * onto the API host so a relative `<img>` does not 404 against Next.js.
 */
export function StoredImage({
  src,
  alt,
  className,
  loading,
}: {
  src: string;
  alt: string;
  className?: string;
  loading?: "eager" | "lazy";
}) {
  const resolved = resolveStoredImageSrc(src);
  if (!resolved) return null;

  // eslint-disable-next-line @next/next/no-img-element -- bytes live on the API host
  return <img src={resolved} alt={alt} className={className} loading={loading} />;
}
