{{/* Common labels applied to every resource. */}}
{{- define "jobshout.labels" -}}
app.kubernetes.io/part-of: jobshout
app.kubernetes.io/managed-by: helm
app.kubernetes.io/instance: {{ .Release.Name }}
jobshout.io/env: {{ .Values.env }}
{{- end -}}

{{/* Selector labels for a given component. Call with (dict "ctx" . "name" "api"). */}}
{{- define "jobshout.selector" -}}
app.kubernetes.io/name: jobshout-{{ .name }}
{{- end -}}

{{/* Fully-qualified image reference for a component (server/web/python-sidecar). */}}
{{- define "jobshout.image" -}}
{{- $ctx := .ctx -}}
{{ $ctx.Values.image.registry }}/{{ $ctx.Values.image.namespace }}/{{ .repo }}:{{ $ctx.Values.image.tag }}
{{- end -}}

{{/* imagePullSecrets block for app pods. */}}
{{- define "jobshout.imagePullSecrets" -}}
{{- if .Values.image.pullSecret }}
imagePullSecrets:
  - name: {{ .Values.image.pullSecret }}
{{- end }}
{{- end -}}
