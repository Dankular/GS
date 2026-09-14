{{- define "gameservice.name" -}}gameservice{{- end }}
{{- define "gameservice.image" -}}
{{- $image := index .Values.images .name -}}
{{- $digest := required (printf "images.%s.digest must be an immutable sha256 digest" .name) $image.digest -}}
{{- if not (regexMatch "^sha256:[0-9a-f]{64}$" $digest) -}}
{{- fail (printf "images.%s.digest must be an immutable sha256 digest" .name) -}}
{{- end -}}
{{- printf "%s@%s" $image.repository $image.digest -}}
{{- end }}
{{- define "gameservice.explicitImage" -}}
{{- $digest := required (printf "%s.image.digest must be an immutable sha256 digest" .name) .image.digest -}}
{{- if not (regexMatch "^sha256:[0-9a-f]{64}$" $digest) -}}
{{- fail (printf "%s.image.digest must be an immutable sha256 digest" .name) -}}
{{- end -}}
{{- printf "%s@%s" .image.repository $digest -}}
{{- end }}
{{- define "gameservice.labels" -}}
app.kubernetes.io/name: gameservice
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
