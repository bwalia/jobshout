{{/* Common labels for JobShout.com marketplace resources. */}}
{{- define "jobshout-com.labels" -}}
app.kubernetes.io/part-of: jobshout-com
app.kubernetes.io/managed-by: helm
app.kubernetes.io/instance: {{ .Release.Name }}
jobshout.com/env: {{ .Values.env }}
{{- end -}}

{{- define "jobshout-com.selector" -}}
app.kubernetes.io/name: jobshout-com-{{ .name }}
{{- end -}}

{{/*
Resolve and validate the image tag.

The platform and the marketplace now share one version line: every v<X.Y.Z> cut
from .unifiedFrom onwards publishes jobshout-com/{api,web} at that same version,
either from a real build or by copying the previous digest onto the new tag. So a
unified v* tag is valid here.

Platform versions OLDER than .unifiedFrom predate that arrangement and have no
images in the jobshout-com namespace — asking for one is how a rollback would
turn into a 15-minute helm --wait on ImagePullBackOff surfacing only as "context
deadline exceeded". Refuse those at template time instead, so the ring fails in
seconds with a message that names the mistake.

jsc-v* is the retired marketplace line, still accepted so old versions redeploy.
*/}}
{{- define "jobshout-com.imageTag" -}}
{{- $tag := .Values.image.tag | toString -}}
{{- $floor := .Values.image.unifiedFrom | toString -}}
{{- if or (hasPrefix "jsc-v" $tag) (eq $tag "latest") -}}
{{- $tag -}}
{{- else if regexMatch "^v[0-9]+\\.[0-9]+\\.[0-9]+$" $tag -}}
{{- if semverCompare (printf ">= %s" (trimPrefix "v" $floor)) (trimPrefix "v" $tag) -}}
{{- $tag -}}
{{- else -}}
{{- fail (printf "image.tag %q predates the unified version line: JobShout.com images are only published from %s onwards, so %s was never pushed to the jobshout-com namespace. Seed a version >= %s, or a jsc-v* version from the retired line." $tag $floor $tag $floor) -}}
{{- end -}}
{{- else -}}
{{- fail (printf "image.tag %q is not a version this chart can deploy. Expected v<X.Y.Z> (>= %s), a jsc-v* version, or latest." $tag $floor) -}}
{{- end -}}
{{- end -}}

{{- define "jobshout-com.image" -}}
{{- $ctx := .ctx -}}
{{ $ctx.Values.image.registry }}/{{ $ctx.Values.image.namespace }}/{{ .repo }}:{{ include "jobshout-com.imageTag" $ctx }}
{{- end -}}

{{- define "jobshout-com.imagePullSecrets" -}}
{{- if .Values.image.pullSecret }}
imagePullSecrets:
  - name: {{ .Values.image.pullSecret }}
{{- end }}
{{- end -}}
