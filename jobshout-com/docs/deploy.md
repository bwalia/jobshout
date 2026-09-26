# JobShout.com — deploy (int / Ring Promoter)

## Public host
- **int:** https://int.jobshout.com
- Ring Promoter: https://rp.workstation.co.uk/?app=jobshout-com

## One-time / automatic edge

Every **int** run of `deploy-jobshout-com.yml` upserts (beaconpulse-style):

1. Cloudflare CNAME `int.jobshout.com` → `lon1.pop0.uk` (`jobshout-com/deploy/scripts/cloudflare-dns.sh`)
2. wslproxy vhost from `jobshout-com/deploy/edge/wslproxy-server-int.json`

Manual fallback:

```bash
gh workflow run register-edge-vhost.yml --repo bwalia/jobshout \
  -f host=int.jobshout.com \
  -f zone=jobshout.com \
  -f server_spec=jobshout-com/deploy/edge/wslproxy-server-int.json \
  -f health_path=/health \
  -f cname_target=lon1.pop0.uk
```

Edge registration is best-effort: the vhost and CNAME outlive any one release, so
a dead admin API warns and the ring still gets its version. Register a genuinely
new host with the manual fallback above.

## Versions — one line, shared with the platform

The marketplace and the platform release on the **same** version. `deploy-k3s.yml`
owns the number and cuts the `vX.Y.Z` tag, then calls `deploy-jobshout-com.yml`
with it. That workflow guarantees `jobshout-com/{api,web}:vX.Y.Z` exists:

- something under `jobshout-com/**` changed → it builds and pushes
- nothing changed → it copies the previous digest onto the new tag
  (`docker buildx imagetools create`, a registry-side manifest write — seconds,
  not the ~11 minutes the emulated web build costs)

So any `vX.Y.Z` is a valid seed for **both** Ring Promoter apps, and seeding one
app with the other's version is no longer a way to lose a quarter of an hour to
`ImagePullBackOff`.

`jsc-v*` is the retired marketplace line. Those tags still deploy — the chart
accepts them — but nothing new is minted on it.

### The floor

Platform versions *older* than `image.unifiedFrom` in `values.yaml` predate this
arrangement and have no images in the `jobshout-com` namespace. The chart refuses
them at template time rather than waiting out a pull that cannot succeed, so a
rollback past the cutover fails in seconds with a message that says why. Raise
the floor only if the marketplace images for a range are ever pruned.

## Continuous deploy
Push to `master` → `deploy-k3s.yml` cuts `vX.Y.Z`, seeds Ring Promoter app
`jobshout`, and calls the marketplace workflow to publish and seed
`jobshout-com` at the same version. Promote in the RP UI.

Marketplace-only re-runs: dispatch `deploy-jobshout-com.yml` with a `VERSION`
(blank = newest release tag) and `DEPLOYMENT_TYPE=deploy` to re-seed without
rebuilding.

## Image builds

`Dockerfile.api` pins its builder to `$BUILDPLATFORM` and cross-compiles to
`$TARGETPLATFORM` (`gcc-<arch>-linux-gnu` + `libc6-dev-<arch>-cross` + the matching
rustup target). Building `linux/amd64` on the arm64 self-hosted runner previously
ran the whole Rust toolchain under QEMU, where gcc's `collect2` segfaulted
intermittently on `zerovec-derive` and a clean build took ~40 minutes. Compiling
natively and emitting foreign object code takes ~3 minutes and does not segfault.

`Dockerfile.web` still builds under emulation (~8 min) — Next.js ships
per-platform SWC binaries in `node_modules`, so the builder has to match the
runtime arch.

Both images are cross-built (no push) on every PR by `pr-image-check.yml`, so a
broken Dockerfile fails the PR instead of the post-merge release run.

## Helm
Chart: `jobshout-com/deploy/helm/jobshout-com`  
Overlays: `values-{int,test,acc,prod}.yaml`  
Release name: `jobshout-com` in shared ring namespaces.

Ring Promoter app entry lives in `bwalia/ring-promoter` ConfigMap
(`deploy/k8s/configmap.yaml`, app name `jobshout-com`).
