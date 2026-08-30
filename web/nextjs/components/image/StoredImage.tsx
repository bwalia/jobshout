"use client";

import { useEffect, useState } from "react";

import { fetchImageObjectURL } from "@/lib/api/images";

/**
 * Renders a stored image URL in the JobShout UI.
 *
 * Generated images live at `/api/v1/images/file/…` and are publicly readable
 * (UUID keys, immutable cache) so opsapi and other consumers can use a plain
 * `<img>`. Inside this app the browser still cannot point `<img>` at the API
 * host with credentials, and relative paths would hit Next.js rather than the
 * Go API — so we fetch the bytes with the session token and render a blob URL.
 *
 * A `data:` source is passed straight through: a freshly generated image comes
 * back inline when there is no object storage, needs no fetching, and would be
 * an odd thing to send to the API to ask for.
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
  const inline = src.startsWith("data:");
  const [objectURL, setObjectURL] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (inline) return;

    // The component can unmount, or src can change, before the bytes arrive.
    // Revoking on the way out matters here: an unrevoked object URL holds a
    // multi-megabyte blob for the life of the document, and a gallery scrolled
    // through for a while would accumulate all of them.
    let live = true;
    let created: string | null = null;

    setObjectURL(null);
    setFailed(false);

    fetchImageObjectURL(src)
      .then((url) => {
        if (!live) {
          URL.revokeObjectURL(url);
          return;
        }
        created = url;
        setObjectURL(url);
      })
      .catch(() => {
        if (live) setFailed(true);
      });

    return () => {
      live = false;
      if (created) URL.revokeObjectURL(created);
    };
  }, [src, inline]);

  if (failed) {
    return (
      <div
        className={`flex items-center justify-center bg-muted text-xs text-muted-foreground ${className ?? ""}`}
        title={alt}
      >
        image unavailable
      </div>
    );
  }

  const resolved = inline ? src : objectURL;
  if (!resolved) {
    // Same box, no content: the layout should not jump when the bytes land.
    return <div className={`animate-pulse bg-muted ${className ?? ""}`} aria-hidden />;
  }

  // eslint-disable-next-line @next/next/no-img-element -- the source is a blob
  // or data URL built in the browser; next/image cannot optimise either.
  return <img src={resolved} alt={alt} className={className} loading={loading} />;
}
