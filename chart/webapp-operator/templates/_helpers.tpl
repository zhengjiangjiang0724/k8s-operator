{{/*
Helper templates shared across the chart.
*/}}

{{/* Chart name. */}}
{{- define "webapp-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Fully qualified app name. Truncated at 63 chars because some Kubernetes
name fields are limited to that.
*/}}
{{- define "webapp-operator.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* Common labels — Kubernetes recommended set. */}}
{{- define "webapp-operator.labels" -}}
app.kubernetes.io/name: {{ include "webapp-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
control-plane: controller-manager
{{- end -}}

{{/* Selector labels — must be stable across upgrades. */}}
{{- define "webapp-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "webapp-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
control-plane: controller-manager
{{- end -}}

{{/* ServiceAccount name. */}}
{{- define "webapp-operator.serviceAccountName" -}}
{{- printf "%s-controller-manager" (include "webapp-operator.fullname" .) -}}
{{- end -}}

{{/* Webhook Service name. */}}
{{- define "webapp-operator.webhookServiceName" -}}
{{- printf "%s-webhook" (include "webapp-operator.fullname" .) -}}
{{- end -}}

{{/* Metrics Service name. */}}
{{- define "webapp-operator.metricsServiceName" -}}
{{- printf "%s-metrics" (include "webapp-operator.fullname" .) -}}
{{- end -}}

{{/* cert-manager Certificate name. */}}
{{- define "webapp-operator.certificateName" -}}
{{- printf "%s-webhook-cert" (include "webapp-operator.fullname" .) -}}
{{- end -}}

{{/* cert-manager Issuer name. */}}
{{- define "webapp-operator.issuerName" -}}
{{- if .Values.certManager.issuerRef.name -}}
{{- .Values.certManager.issuerRef.name -}}
{{- else -}}
{{- printf "%s-selfsigned" (include "webapp-operator.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/* TLS Secret name produced by cert-manager. */}}
{{- define "webapp-operator.webhookCertSecretName" -}}
{{- printf "%s-webhook-server-cert" (include "webapp-operator.fullname" .) -}}
{{- end -}}
