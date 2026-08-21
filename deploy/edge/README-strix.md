# Edge: strix.workstation.co.uk

Register this host on pop0 the same way `images.workstation.co.uk` was wired —
upstream the Mac Studio on port **11436**, JWT gateway on, health path `/health`.

DNS may already resolve (zone CNAME to `pop0.wslproxy.com`). Until the vhost
exists, the edge answers "Host not configured".

The checked-in `register-edge-vhost.yml` workflow targets `jobshout.co.uk` app
hosts (k3s Ingress). Workstation services (`ollama` / `images` / `strix` on
`workstation.co.uk`) use the wslproxy admin API / dashboard with an upstream to
the workstation, not those app server specs.

After registration, confirm:

```bash
curl -sS https://strix.workstation.co.uk/health
```

Then `/api/capabilities` with a hand-minted `x-api-key` JWT whose secret matches
`STRIX_JWT_SECRET` in the ring (distinct from `IMAGE_JWT_SECRET`).
