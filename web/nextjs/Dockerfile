# Build on the builder's architecture, ship for the target's — the same split
# server/Dockerfile already uses for Go.
#
# k3s1 is amd64 and the CI runner is an arm64 Mac, so this image is always a
# cross-build. Running `npm run build` under QEMU emulation took ~28 minutes and
# then produced no output at all while still exiting 0, which surfaced only as a
# confusing "/app/.next/standalone: not found" two stages later. That failure
# blocked every deploy from PR #29 onward.
#
# Building natively avoids the emulator entirely. It is safe here because Next's
# standalone output is pure JavaScript: nothing native is traced into it — no
# .node binaries, no platform-specific packages — so it runs unchanged on amd64.
# Only the runtime stage below needs the target architecture, and that stage is
# pulled rather than built. If a native dependency is ever added (sharp is the
# usual one), this assumption breaks and the build stages must go back to the
# target platform.
FROM --platform=$BUILDPLATFORM node:20-alpine AS deps
RUN apk add --no-cache libc6-compat
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci || npm install

FROM --platform=$BUILDPLATFORM node:20-alpine AS builder
WORKDIR /app
ARG NEXT_PUBLIC_API_URL
ARG NEXT_PUBLIC_WS_URL
ENV NEXT_PUBLIC_API_URL=${NEXT_PUBLIC_API_URL}
ENV NEXT_PUBLIC_WS_URL=${NEXT_PUBLIC_WS_URL}
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build
# `next build` exiting 0 is not proof it wrote anything — that is exactly how
# the emulated build failed. Assert the outputs here, where the message names
# the stage that was supposed to produce them, instead of letting the COPY in
# the next stage report a missing path and leave the cause to guesswork.
RUN test -d .next/standalone && test -d .next/static

# Runtime: the target architecture. Pulled, never built.
FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static


USER nextjs
EXPOSE 3001
ENV PORT=3001
ENV HOSTNAME="0.0.0.0"

CMD ["node", "server.js"]
