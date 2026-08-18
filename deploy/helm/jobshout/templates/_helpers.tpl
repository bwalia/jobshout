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

{{/*
Environment shared by langfuse-web and langfuse-worker. Secrets come from the
in-cluster generated jobshout-langfuse-secrets (see langfuse-secrets.yaml);
DATABASE_URL is assembled with k8s $(VAR) expansion so the password never
appears in the pod spec.
*/}}
{{- define "jobshout.langfuseSharedEnv" -}}
- name: LANGFUSE_DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: jobshout-langfuse-secrets
      key: LANGFUSE_DB_PASSWORD
- name: DATABASE_URL
  value: "postgresql://langfuse:$(LANGFUSE_DB_PASSWORD)@langfuse-postgres:5432/langfuse"
- name: NEXTAUTH_URL
  value: "https://{{ .Values.langfuse.host }}"
- name: SALT
  valueFrom:
    secretKeyRef:
      name: jobshout-langfuse-secrets
      key: SALT
- name: ENCRYPTION_KEY
  valueFrom:
    secretKeyRef:
      name: jobshout-langfuse-secrets
      key: ENCRYPTION_KEY
- name: TELEMETRY_ENABLED
  value: "false"
- name: CLICKHOUSE_MIGRATION_URL
  value: "clickhouse://langfuse-clickhouse:9000"
- name: CLICKHOUSE_URL
  value: "http://langfuse-clickhouse:8123"
- name: CLICKHOUSE_USER
  value: "clickhouse"
- name: CLICKHOUSE_PASSWORD
  valueFrom:
    secretKeyRef:
      name: jobshout-langfuse-secrets
      key: CLICKHOUSE_PASSWORD
- name: CLICKHOUSE_CLUSTER_ENABLED
  value: "false"
- name: REDIS_HOST
  value: "langfuse-redis"
- name: REDIS_PORT
  value: "6379"
- name: REDIS_AUTH
  valueFrom:
    secretKeyRef:
      name: jobshout-langfuse-secrets
      key: REDIS_AUTH
- name: LANGFUSE_S3_EVENT_UPLOAD_BUCKET
  value: "langfuse"
- name: LANGFUSE_S3_EVENT_UPLOAD_REGION
  value: "auto"
- name: LANGFUSE_S3_EVENT_UPLOAD_ACCESS_KEY_ID
  value: "langfuse"
- name: LANGFUSE_S3_EVENT_UPLOAD_SECRET_ACCESS_KEY
  valueFrom:
    secretKeyRef:
      name: jobshout-langfuse-secrets
      key: MINIO_ROOT_PASSWORD
- name: LANGFUSE_S3_EVENT_UPLOAD_ENDPOINT
  value: "http://langfuse-minio:9000"
- name: LANGFUSE_S3_EVENT_UPLOAD_FORCE_PATH_STYLE
  value: "true"
- name: LANGFUSE_S3_EVENT_UPLOAD_PREFIX
  value: "events/"
# Media upload points in-cluster: the sidecar never sends multimodal payloads,
# and the presigned-URL-in-browser path (which would need a public MinIO host)
# is unused here.
- name: LANGFUSE_S3_MEDIA_UPLOAD_BUCKET
  value: "langfuse"
- name: LANGFUSE_S3_MEDIA_UPLOAD_REGION
  value: "auto"
- name: LANGFUSE_S3_MEDIA_UPLOAD_ACCESS_KEY_ID
  value: "langfuse"
- name: LANGFUSE_S3_MEDIA_UPLOAD_SECRET_ACCESS_KEY
  valueFrom:
    secretKeyRef:
      name: jobshout-langfuse-secrets
      key: MINIO_ROOT_PASSWORD
- name: LANGFUSE_S3_MEDIA_UPLOAD_ENDPOINT
  value: "http://langfuse-minio:9000"
- name: LANGFUSE_S3_MEDIA_UPLOAD_FORCE_PATH_STYLE
  value: "true"
- name: LANGFUSE_S3_MEDIA_UPLOAD_PREFIX
  value: "media/"
{{- end -}}
