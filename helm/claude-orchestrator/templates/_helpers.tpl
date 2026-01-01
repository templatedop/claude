{{/*
Expand the name of the chart.
*/}}
{{- define "claude-orchestrator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "claude-orchestrator.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "claude-orchestrator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "claude-orchestrator.labels" -}}
helm.sh/chart: {{ include "claude-orchestrator.chart" . }}
{{ include "claude-orchestrator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "claude-orchestrator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "claude-orchestrator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Worker labels
*/}}
{{- define "claude-orchestrator.worker.labels" -}}
{{ include "claude-orchestrator.labels" . }}
app.kubernetes.io/component: worker
{{- end }}

{{/*
Worker selector labels
*/}}
{{- define "claude-orchestrator.worker.selectorLabels" -}}
{{ include "claude-orchestrator.selectorLabels" . }}
app.kubernetes.io/component: worker
{{- end }}

{{/*
API labels
*/}}
{{- define "claude-orchestrator.api.labels" -}}
{{ include "claude-orchestrator.labels" . }}
app.kubernetes.io/component: api
{{- end }}

{{/*
API selector labels
*/}}
{{- define "claude-orchestrator.api.selectorLabels" -}}
{{ include "claude-orchestrator.selectorLabels" . }}
app.kubernetes.io/component: api
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "claude-orchestrator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "claude-orchestrator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the secrets
*/}}
{{- define "claude-orchestrator.secretName" -}}
{{- if .Values.secrets.create }}
{{- include "claude-orchestrator.fullname" . }}-secrets
{{- else }}
{{- .Values.secrets.existingSecret }}
{{- end }}
{{- end }}

{{/*
Get the Anthropic API key secret name
*/}}
{{- define "claude-orchestrator.anthropicSecretName" -}}
{{- if .Values.secrets.anthropicApiKeyExistingSecret }}
{{- .Values.secrets.anthropicApiKeyExistingSecret }}
{{- else }}
{{- include "claude-orchestrator.secretName" . }}
{{- end }}
{{- end }}

{{/*
Get the OpenAI API key secret name
*/}}
{{- define "claude-orchestrator.openaiSecretName" -}}
{{- if .Values.secrets.openaiApiKeyExistingSecret }}
{{- .Values.secrets.openaiApiKeyExistingSecret }}
{{- else }}
{{- include "claude-orchestrator.secretName" . }}
{{- end }}
{{- end }}

{{/*
Get Temporal address
*/}}
{{- define "claude-orchestrator.temporalAddress" -}}
{{- if .Values.temporal.enabled }}
{{- printf "%s-frontend:7233" (include "claude-orchestrator.fullname" .) }}
{{- else }}
{{- .Values.config.temporal.address }}
{{- end }}
{{- end }}

{{/*
Get PostgreSQL host
*/}}
{{- define "claude-orchestrator.postgresHost" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-postgresql" (include "claude-orchestrator.fullname" .) }}
{{- else }}
{{- .Values.externalPostgresql.host }}
{{- end }}
{{- end }}

{{/*
Get MinIO endpoint
*/}}
{{- define "claude-orchestrator.minioEndpoint" -}}
{{- if .Values.minio.enabled }}
{{- printf "http://%s-minio:9000" (include "claude-orchestrator.fullname" .) }}
{{- else }}
{{- .Values.externalStorage.endpoint }}
{{- end }}
{{- end }}

{{/*
Worker image
*/}}
{{- define "claude-orchestrator.worker.image" -}}
{{- $registry := .Values.global.imageRegistry | default "" }}
{{- $repository := .Values.worker.image.repository }}
{{- $tag := .Values.worker.image.tag | default .Chart.AppVersion }}
{{- if $registry }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- else }}
{{- printf "%s:%s" $repository $tag }}
{{- end }}
{{- end }}

{{/*
API image
*/}}
{{- define "claude-orchestrator.api.image" -}}
{{- $registry := .Values.global.imageRegistry | default "" }}
{{- $repository := .Values.api.image.repository }}
{{- $tag := .Values.api.image.tag | default .Chart.AppVersion }}
{{- if $registry }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- else }}
{{- printf "%s:%s" $repository $tag }}
{{- end }}
{{- end }}
