{{- define "image-warmer.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 48 | trimSuffix "-" -}}
{{- end -}}

{{- define "image-warmer.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 48 | trimSuffix "-" -}}
{{- else if contains (include "image-warmer.name" .) .Release.Name -}}
{{- .Release.Name | trunc 48 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "image-warmer.name" .) | trunc 48 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "image-warmer.clusterName" -}}
{{- printf "%s-%s" .Release.Namespace (include "image-warmer.fullname" .) | trunc 54 | trimSuffix "-" -}}
{{- end -}}

{{- define "image-warmer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "image-warmer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "image-warmer.labels" -}}
{{ include "image-warmer.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | quote }}
{{- end -}}

{{- define "image-warmer.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "image-warmer.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- required "serviceAccount.name is required when serviceAccount.create=false" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "image-warmer.policyName" -}}
{{- default (include "image-warmer.clusterName" .) .Values.policy.name -}}
{{- end -}}

{{- define "image-warmer.image" -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) -}}
{{- end -}}
{{- end -}}
