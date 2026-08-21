# Edge: strix.workstation.co.uk

Register this host on **pop1** (not pop0). pop0 is a k3s/traefik node; the
workstation services (Ollama, images, strix) terminate on the pop1 edge.

Upstream the Mac Studio on port **11436**, JWT gateway on, health path `/health`.
Clone the live `images.workstation.co.uk` vhost (port 11435) and change the
origin port — same JWT scheme, adjacent port.

DNS should be a CNAME to `pop1.wslproxy.com`. Until the vhost exists, the edge
answers "Host not configured".

Register via:

```bash
gh workflow run register-edge-vhost.yml --repo bwalia/jobshout \
  -f host=strix.workstation.co.uk \
  -f zone=workstation.co.uk \
  -f server_spec=deploy/edge/wslproxy-server-strix.json \
  -f health_path=/health \
  -f cname_target=pop1.wslproxy.com \
  -f gateway_url=https://pop1.diytaxreturn.co.uk
```

After registration:

```bash
curl -sS https://strix.workstation.co.uk/health
# JWT-gated: 403 with x-api-key is success (vhost exists).
# "Host not configured" means the vhost is still missing.
```

Then `/api/capabilities` with a hand-minted `x-api-key` JWT whose secret matches
`STRIX_JWT_SECRET` in the ring (distinct from `IMAGE_JWT_SECRET`).
