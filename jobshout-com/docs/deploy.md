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

## Continuous deploy
Push to `master` under `jobshout-com/**` → builds `jobshout-com/{api,web}:jsc-v…`
→ seeds Ring Promoter int. Promote in the RP UI.

## Helm
Chart: `jobshout-com/deploy/helm/jobshout-com`  
Overlays: `values-{int,test,acc,prod}.yaml`  
Release name: `jobshout-com` in shared ring namespaces.

Ring Promoter app entry lives in `bwalia/ring-promoter` ConfigMap
(`deploy/k8s/configmap.yaml`, app name `jobshout-com`).
