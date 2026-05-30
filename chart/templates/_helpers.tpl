{{/*
Expand the name of the chart.
*/}}
{{- define "darb.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "darb.fullname" -}}
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
{{- define "darb.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "darb.labels" -}}
helm.sh/chart: {{ include "darb.chart" . }}
{{ include "darb.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "darb.selectorLabels" -}}
app.kubernetes.io/name: {{ include "darb.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "darb.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "darb.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Generate the namespace name
*/}}
{{- define "darb.namespace" -}}
{{- default .Release.Name .Values.namespaceOverride }}
{{- end }}

{{/*
Backwards-compatible helper aliases.
Existing templates reference "nebi.*" names, so map them to darb.* helpers.
*/}}
{{- define "nebi.name" -}}
{{ include "darb.name" . }}
{{- end }}

{{- define "nebi.fullname" -}}
{{ include "darb.fullname" . }}
{{- end }}

{{- define "nebi.chart" -}}
{{ include "darb.chart" . }}
{{- end }}

{{- define "nebi.labels" -}}
{{ include "darb.labels" . }}
{{- end }}

{{- define "nebi.selectorLabels" -}}
{{ include "darb.selectorLabels" . }}
{{- end }}

{{- define "nebi.serviceAccountName" -}}
{{ include "darb.serviceAccountName" . }}
{{- end }}

{{- define "nebi.namespace" -}}
{{ include "darb.namespace" . }}
{{- end }}
