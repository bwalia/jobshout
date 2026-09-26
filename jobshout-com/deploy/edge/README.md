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
