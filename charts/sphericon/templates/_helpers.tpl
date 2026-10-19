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

{{/* Non-empty when the secrets come as environment variables (existingSecret), the default. */}}
{{- define "sphericon.envSecret" -}}
{{- if or .Values.existingSecret (not .Values.secretFiles.secretName) }}true{{ end }}
{{- end -}}

{{- define "sphericon.envFrom" -}}
- secretRef:
    name: {{ required "existingSecret or secretFiles.secretName is required" .Values.existingSecret }}
{{- end -}}

{{/* Where the secretFiles Secret is mounted. */}}
{{- define "sphericon.secretFilesDir" -}}
/run/secrets/sphericon
{{- end -}}

{{/* The _FILE variables pointing at the mounted secretFiles Secret. */}}
{{- define "sphericon.secretFileEnv" -}}
{{- range .Values.secretFiles.keys }}
- name: {{ printf "%s_FILE" . }}
  value: {{ printf "%s/%s" (include "sphericon.secretFilesDir" $) . | quote }}
{{- end }}
{{- end -}}

{{- define "sphericon.secretFileMount" -}}
- name: secret-files
  mountPath: {{ include "sphericon.secretFilesDir" . }}
  readOnly: true
{{- end -}}

{{- define "sphericon.secretFileVolume" -}}
- name: secret-files
  secret:
    secretName: {{ .Values.secretFiles.secretName | quote }}
{{- end -}}
