{{- define "omada2pihole.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "omada2pihole.fullname" -}}
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

{{- define "omada2pihole.selectorLabels" -}}
app.kubernetes.io/name: {{ include "omada2pihole.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "omada2pihole.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "omada2pihole.selectorLabels" . }}
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "omada2pihole.secretName" -}}
{{- .Values.credentials.existingSecret | default (include "omada2pihole.fullname" .) }}
{{- end }}

{{- define "omada2pihole.image" -}}
{{- if .Values.image.digest }}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest }}
{{- else }}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) }}
{{- end }}
{{- end }}

{{/*
Validate the settings that have no application default. With an existing Secret they
may live in the Secret instead, so they are not checked.
*/}}
{{- define "omada2pihole.validate" -}}
{{- if not .Values.credentials.existingSecret }}
{{- $c := .Values.config }}
{{- $missing := list }}
{{- if not $c.omada.url }}{{ $missing = append $missing "config.omada.url" }}{{ end }}
{{- if not $c.omada.id }}{{ $missing = append $missing "config.omada.id" }}{{ end }}
{{- if not $c.pihole.url }}{{ $missing = append $missing "config.pihole.url" }}{{ end }}
{{- if not .Values.credentials.omadaClientId }}{{ $missing = append $missing "credentials.omadaClientId" }}{{ end }}
{{- if not .Values.credentials.omadaClientSecret }}{{ $missing = append $missing "credentials.omadaClientSecret" }}{{ end }}
{{- if $missing }}
{{- fail (printf "missing values: %s (or set credentials.existingSecret)" (join ", " $missing)) }}
{{- end }}
{{- end }}
{{- end }}

{{/* Container spec shared by the CronJob and the Deployment. Pass a dict with ctx and args. */}}
{{- define "omada2pihole.podSpec" -}}
{{- $ := .ctx -}}
{{- $c := $.Values.config -}}
{{- $env := dict
  "OMADA_URL" $c.omada.url
  "OMADA_ID" $c.omada.id
  "OMADA_SITE" $c.omada.site
  "OMADA_CA_FILE" $c.omada.caFile
  "PIHOLE_URL" $c.pihole.url
  "PIHOLE_CA_FILE" $c.pihole.caFile
  "DOMAIN_SUFFIX" $c.domainSuffix
  "LOG_LEVEL" $c.logLevel
-}}
{{- $flags := dict
  "OMADA_TLS_SKIP_VERIFY" $c.omada.tlsSkipVerify
  "PIHOLE_TLS_SKIP_VERIFY" $c.pihole.tlsSkipVerify
  "PIHOLE_RESTART_DNS" $c.pihole.restartDns
  "DRY_RUN" $c.dryRun
  "ALLOW_MASS_DELETE" $c.allowMassDelete
-}}
{{- with $.Values.imagePullSecrets }}
imagePullSecrets:
  {{- toYaml . | nindent 2 }}
{{- end }}
automountServiceAccountToken: false
{{- with $.Values.podSecurityContext }}
securityContext:
  {{- toYaml . | nindent 2 }}
{{- end }}
containers:
  - name: sync
    image: {{ include "omada2pihole.image" $ | quote }}
    imagePullPolicy: {{ $.Values.image.pullPolicy }}
    args:
      {{- toYaml .args | nindent 6 }}
    envFrom:
      - secretRef:
          name: {{ include "omada2pihole.secretName" $ }}
    {{- $hasEnv := $.Values.extraEnv }}
    {{- range $k, $v := $env }}{{ if $v }}{{ $hasEnv = true }}{{ end }}{{ end }}
    {{- range $k, $v := $flags }}{{ if $v }}{{ $hasEnv = true }}{{ end }}{{ end }}
    {{- if $hasEnv }}
    env:
      {{- range $k, $v := $env }}
      {{- if $v }}
      - name: {{ $k }}
        value: {{ $v | toString | quote }}
      {{- end }}
      {{- end }}
      {{- range $k, $v := $flags }}
      {{- if $v }}
      - name: {{ $k }}
        value: "true"
      {{- end }}
      {{- end }}
      {{- with $.Values.extraEnv }}
      {{- toYaml . | nindent 6 }}
      {{- end }}
    {{- end }}
    {{- with $.Values.securityContext }}
    securityContext:
      {{- toYaml . | nindent 6 }}
    {{- end }}
    {{- with $.Values.resources }}
    resources:
      {{- toYaml . | nindent 6 }}
    {{- end }}
    {{- with $.Values.extraVolumeMounts }}
    volumeMounts:
      {{- toYaml . | nindent 6 }}
    {{- end }}
{{- with $.Values.extraVolumes }}
volumes:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with $.Values.nodeSelector }}
nodeSelector:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with $.Values.tolerations }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with $.Values.affinity }}
affinity:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{- define "omada2pihole.podMetadata" -}}
labels:
  {{- include "omada2pihole.selectorLabels" . | nindent 2 }}
  {{- with .Values.podLabels }}
  {{- toYaml . | nindent 2 }}
  {{- end }}
{{- $ann := deepCopy (.Values.podAnnotations | default dict) }}
{{- if not .Values.credentials.existingSecret }}
{{- $_ := set $ann "checksum/secret" (include (print .Template.BasePath "/secret.yaml") . | sha256sum) }}
{{- end }}
{{- with $ann }}
annotations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}
