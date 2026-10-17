# JobShout.com edge registration

Public host for the int ring: **https://int.jobshout.com**

Same edge path as the platform (`int.jobshout.co.uk`):

1. Cloudflare CNAME `int.jobshout.com` → `lon1.pop0.uk` (zone `jobshout.com`)
2. wslproxy vhost on pop0 using Traefik rule `8f161403-8592-1111-6294-9c57974505b0`

```bash
gh workflow run register-edge-vhost.yml --repo bwalia/jobshout \
  -f host=int.jobshout.com \
  -f zone=jobshout.com \
  -f server_spec=jobshout-com/deploy/edge/wslproxy-server-int.json \
  -f health_path=/health \
  -f cname_target=lon1.pop0.uk
```

Deploy / promote: https://rp.workstation.co.uk/?app=jobshout-com

## int and test are VPN / home-LAN only

`int.jobshout.com` and `test.jobshout.com` carry
`"access_profile": "jobshout-preprod"`. wslproxy expands it into an allow/deny
rule pair on `/` (see wslproxy `docs/VPN_ACCESS.md`):

| Who | Source IP seen by pop0 | Result |
|-----|------------------------|--------|
| LAN GitHub runners (debian00x, mac-studio-01) | home public IP `193.237.176.232` | proxied |
| WireGuard clients (`AllowedIPs` includes `195.20.255.201/32`) | `193.237.176.232` (NAT'd by the hub) | proxied |
| Everyone else, incl. GitHub-hosted `ubuntu-latest` | their own | 403 "VPN required" |

Let's Encrypt keeps working: `/.well-known/acme-challenge/` is a dedicated nginx
`location` on pop0, served before any rule runs. DNS stays public for the same
reason.

### The profile lives on pop0, not behind the API

wslproxy loads profiles from disk only. The source of truth is
[`access-profiles/prod/jobshout-preprod.json`](access-profiles/prod/jobshout-preprod.json);
copy it to pop0 **before** a vhost references it:

```bash
scp jobshout-com/deploy/edge/access-profiles/prod/jobshout-preprod.json \
  root@195.20.255.201:/opt/nginx/data/access_profiles/prod/
```

Order matters. A vhost naming a profile that is missing or invalid fails
**closed** — every request gets the 403 page, logged on pop0 as
`access_profile failed, denying all requests for <host>`.

`origin` is set explicitly to the same backend as rule `8f161403` (Traefik on
`:8888`). Without it the allow rule falls back to the vhost's `proxy_pass`,
which is `http://localhost`.

### Known limits

- **Dynamic home IP.** `193.237.176.232` is residential. When it changes, update
  `allow_cidrs` and `origin` in the profile (and rule `8f161403`) and re-copy it.
- **Whole home LAN is allowed**, not just WireGuard peers.
- **Origin bypass.** `193.237.176.232:8888` is port-forwarded to Traefik; restrict
  that forward to pop0 (`195.20.255.201`) or this profile can be sidestepped.
- `register-edge-vhost.yml`'s health probe runs on `ubuntu-latest` and will see
  403 for these hosts — expected.
