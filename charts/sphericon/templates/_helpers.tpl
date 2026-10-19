{{- define "sphericon.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "sphericon.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "sphericon.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "sphericon.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}
{{- end -}}

{{/* Shared container environment: plain settings (secrets come from envFrom). */}}
{{- define "sphericon.env" -}}
- name: SPHERICON_ENV
  value: production
- name: PORT
  value: {{ .Values.containerPort | quote }}
- name: SPHERICON_DB_AUTO_MIGRATE
  value: "false"
{{- with .Values.appUrl }}
- name: SPHERICON_APP_URL
  value: {{ . | quote }}
{{- end }}
{{- if .Values.metrics.enabled }}
- name: SPHERICON_METRICS_ADDR
  value: {{ printf "0.0.0.0:%d" (int .Values.metrics.port) | quote }}
{{- end }}
{{- include "sphericon.secretFileEnv" . }}
{{- with .Values.extraEnv }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/* Secrets as environment variables; empty when only secretFiles supplies them. */}}
{{- define "sphericon.envFrom" -}}
{{- if or .Values.existingSecret (not .Values.secretFiles.secretName) }}
- secretRef:
    name: {{ required "existingSecret or secretFiles.secretName is required" .Values.existingSecret }}
{{- else }}
[]
{{- end }}
{{- end -}}

{{/* The _FILE variables pointing at the mounted secretFiles Secret. */}}
{{- define "sphericon.secretFileEnv" -}}
{{- range .Values.secretFiles.keys }}
- name: {{ printf "%s_FILE" . }}
  value: {{ printf "/run/secrets/sphericon/%s" . | quote }}
{{- end }}
{{- end -}}

{{- define "sphericon.secretFileMount" -}}
{{- if .Values.secretFiles.secretName }}
- name: secret-files
  mountPath: /run/secrets/sphericon
  readOnly: true
{{- end }}
{{- end -}}

{{- define "sphericon.secretFileVolume" -}}
{{- if .Values.secretFiles.secretName }}
- name: secret-files
  secret:
    secretName: {{ .Values.secretFiles.secretName | quote }}
{{- end }}
{{- end -}}
