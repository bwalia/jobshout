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
Marketplace images are only ever published as jsc-vX.Y.Z (or :latest) under the
jobshout-com registry namespace. The platform's own v* tags live in the jobshout
namespace, so a v1.0.x tag here resolves to an image that will never exist and
helm --wait sits on ImagePullBackOff until it reports "context deadline
exceeded". Refuse the tag at template time instead, so the ring fails in seconds
with a message that names the mistake.
*/}}
{{- define "jobshout-com.imageTag" -}}
{{- $tag := .Values.image.tag | toString -}}
{{- if not (or (hasPrefix "jsc-v" $tag) (eq $tag "latest")) -}}
{{- fail (printf "image.tag %q is not a JobShout.com version. Marketplace images are tagged jsc-vX.Y.Z; platform v* tags belong to Ring Promoter app \"jobshout\", not \"jobshout-com\"." $tag) -}}
{{- end -}}
{{- $tag -}}
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
