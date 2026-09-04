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

{{- define "jobshout-com.image" -}}
{{- $ctx := .ctx -}}
{{ $ctx.Values.image.registry }}/{{ $ctx.Values.image.namespace }}/{{ .repo }}:{{ $ctx.Values.image.tag }}
{{- end -}}

{{- define "jobshout-com.imagePullSecrets" -}}
{{- if .Values.image.pullSecret }}
imagePullSecrets:
  - name: {{ .Values.image.pullSecret }}
{{- end }}
{{- end -}}
