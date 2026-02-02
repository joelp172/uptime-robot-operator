{{/*
Expand the name of the chart.
*/}}
{{- define "uptime-robot-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "uptime-robot-operator.fullname" -}}
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
{{- define "uptime-robot-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "uptime-robot-operator.labels" -}}
helm.sh/chart: {{ include "uptime-robot-operator.chart" . }}
{{ include "uptime-robot-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "uptime-robot-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "uptime-robot-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
control-plane: controller-manager
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "uptime-robot-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "uptime-robot-operator.fullname" . | printf "%s-controller-manager") .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the namespace to use
*/}}
{{- define "uptime-robot-operator.namespace" -}}
{{- default "uptime-robot-system" .Values.namespaceOverride }}
{{- end }}

{{/*
Create the image string
*/}}
{{- define "uptime-robot-operator.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion | default "latest" }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}
