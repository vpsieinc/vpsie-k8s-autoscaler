{{/*
Expand the name of the chart.
*/}}
{{- define "vpsie-autoscaler.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "vpsie-autoscaler.fullname" -}}
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
{{- define "vpsie-autoscaler.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "vpsie-autoscaler.labels" -}}
helm.sh/chart: {{ include "vpsie-autoscaler.chart" . }}
{{ include "vpsie-autoscaler.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "vpsie-autoscaler.selectorLabels" -}}
app.kubernetes.io/name: {{ include "vpsie-autoscaler.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "vpsie-autoscaler.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "vpsie-autoscaler.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the appropriate apiVersion for RBAC APIs.
*/}}
{{- define "vpsie-autoscaler.rbac.apiVersion" -}}
{{- if .Capabilities.APIVersions.Has "rbac.authorization.k8s.io/v1" -}}
rbac.authorization.k8s.io/v1
{{- else -}}
rbac.authorization.k8s.io/v1beta1
{{- end -}}
{{- end -}}

{{/*
Return the appropriate apiVersion for PodDisruptionBudget.
*/}}
{{- define "vpsie-autoscaler.pdb.apiVersion" -}}
{{- if .Capabilities.APIVersions.Has "policy/v1/PodDisruptionBudget" -}}
policy/v1
{{- else -}}
policy/v1beta1
{{- end -}}
{{- end -}}

{{/*
Get the VPSie secret name
*/}}
{{- define "vpsie-autoscaler.secretName" -}}
{{- if .Values.vpsie.existingSecret }}
{{- .Values.vpsie.existingSecret }}
{{- else }}
{{- include "vpsie-autoscaler.fullname" . }}-vpsie-secret
{{- end }}
{{- end }}

{{/*
Get the webhook service name
*/}}
{{- define "vpsie-autoscaler.webhook.serviceName" -}}
{{- printf "%s-webhook" (include "vpsie-autoscaler.fullname" .) }}
{{- end }}

{{/*
Get the webhook certificate secret name
*/}}
{{- define "vpsie-autoscaler.webhook.certSecretName" -}}
{{- printf "%s-webhook-cert" (include "vpsie-autoscaler.fullname" .) }}
{{- end }}
