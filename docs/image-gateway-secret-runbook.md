# Runbook: the image gateway secret

`images.workstation.co.uk` fronts the workstation image service, which also
renders the Video Producer's videos. It uses the same three-gate pattern as the
Strix gateway ([pentest-gateway-secret-runbook.md](pentest-gateway-secret-runbook.md)):
**one HS256 secret, byte-identical in three places**. If any gate drifts, images
and videos fail. The symptom on int:

```
video: resume check skipped — workstation unreachable
  error: the workstation gateway rejected the request (status 403)
```

| Gate | Where the secret lives | Role |
| --- | --- | --- |
| **rings** (int/test/acc/prod) | k8s secret `jobshout-gateways`, key `IMAGE_JWT_SECRET` (shared value, hand-made per namespace) | signs the token (`x-api-key`) |
| **edge** (wslproxy, admin `https://lon1.pop0.uk`) | server `images.workstation.co.uk` → rule `6bd45a55-7130-4285-a823-63999a4ec386` → `match.rules.jwt_token_validation_key` | verifies the signature |
| **workstation** | launchd plist `com.jobshout.image-service` → `EnvironmentVariables.IMAGE_JWT_SECRET` | verifies the signature again (blocks direct hits on the forwarded port) |

The rings' value is the canonical one. Change the edge and the workstation to
match it, never the other way round.

## Health check

On the workstation, int's boot leaves two lines about 30 s apart:

```bash
grep "195.20.255.201" ~/Library/Logs/jobshout/image-service.log | tail -3
# "GET /api/models HTTP/1.1" 200     ← image warm-up
# "GET /api/videos HTTP/1.1" 200     ← video resumer
```

`195.20.255.201` is the edge (`pop0.wslproxy.com`), not int. Every ring reaches
the workstation through it. `401` here means the workstation gate drifted. If
nothing arrives at all, the edge rejected the request (check int's log for the
403 above).

Unsigned requests must be refused:

```bash
curl -s localhost:11435/                       # "authenticated": true
curl -s -o /dev/null -w "%{http_code}\n" localhost:11435/api/videos   # 401
```

## Fix the edge (zsh-safe; prints lengths, never the secret)

```zsh
GW=https://lon1.pop0.uk
read "U?wslproxy email: "; read -s "P?wslproxy password: "; echo
TOKEN=$(curl -fsS -X POST "$GW/api/user/login" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg e "$U" --arg p "$P" '{email:$e,password:$p}')" | jq -r '.data.accessToken')
SERVERS=$(curl -fsS "$GW/api/servers?limit=500" -H "Authorization: Bearer $TOKEN")
VAL=$(kubectl -n int get secret jobshout-gateways -o jsonpath='{.data.IMAGE_JWT_SECRET}' | base64 -d)
for rid in $(printf '%s' "$SERVERS" | jq -r '.data[] | select(.server_name=="images.workstation.co.uk") | .rules | if type=="array" then .[] elif type=="string" then . else empty end'); do
  RULE=$(curl -fsS "$GW/api/rules/$rid" -H "Authorization: Bearer $TOKEN" | jq '(.data//.)')
  KEY=$(printf '%s' "$RULE" | jq -r '.match.rules.jwt_token_validation_key // ""')
  echo "rule $rid key_len=${#KEY} matches=$([ "$KEY" = "$VAL" ] && echo yes || echo NO)"
  [ -n "$VAL" ] && [ "$KEY" != "$VAL" ] && printf '%s' "$RULE" | jq --arg k "$VAL" '.match.rules.jwt_token_validation_key=$k' \
    | curl -sS -X PUT "$GW/api/rules/$rid" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d @- -o /dev/null -w "updated -> %{http_code}\n"
done
unset U P TOKEN VAL KEY RULE SERVERS
```

Then click **Sync** (the cloud icon) in the console. **Never "Save Changes" on the
server**: it rewrites the key.

## Fix the workstation

```zsh
kubectl -n int get secret jobshout-gateways -o jsonpath='{.data.IMAGE_JWT_SECRET}' | base64 -d \
 | ssh -p 2225 balinderwalia@hh193.workstation.co.uk 'v=$(cat); [ -n "$v" ] || exit 1; L=com.jobshout.image-service; P=~/Library/LaunchAgents/$L.plist;
   plutil -replace EnvironmentVariables.IMAGE_JWT_SECRET -string "$v" $P;
   launchctl bootout gui/$(id -u)/$L; sleep 2; launchctl bootstrap gui/$(id -u) $P; launchctl kickstart gui/$(id -u)/$L; echo "applied (${#v})"'
```

Use **bootout + bootstrap**, not `kickstart -k`. `kickstart` restarts the
definition launchd loaded earlier, so a changed environment variable is ignored
until the job is reloaded.

## Gotchas (learned 2026-10-13)

- **zsh `echo` corrupts JSON.** It turns `\n` inside JSON strings into real
  newlines, and `jq` then rejects the input. A PUT built that way sends a broken
  rule. Use `printf '%s' "$VAR"`.
- **zsh `read -p` is not a prompt** (it reads from a coprocess). Use
  `read "VAR?prompt: "`.
- **A wslproxy server's `rules` is sometimes one string, not a list.**
  `images.workstation.co.uk` has a string; `strix.workstation.co.uk` has a list.
  `.rules[]` silently finds nothing on a string.
- **A broken image gateway is silent on int**, because int makes images with
  Gemini first. The last edge-routed generation before this fix was 27 Sep.
  The Video Producer's boot check (above) is now the canary.

## Known-good state (2026-10-13)

All three gates hold the 32-character `jobshout-gateways` value. The edge rule
previously held a stale 44-character key. The workstation previously had
**no** secret, while its port (forwarded) was reachable from the internet;
scanners appeared in its log. int → edge → workstation verified: signed
`GET /api/videos` 200, unsigned 401.
