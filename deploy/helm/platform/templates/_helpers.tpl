{{- define "gameservice.name" -}}gameservice{{- end }}
{{- define "gameservice.image" -}}
{{- $image := index .Values.images .name -}}
{{- required (printf "images.%s.digest must be an immutable sha256 digest" .name) $image.digest -}}
{{- printf "%s@%s" $image.repository $image.digest -}}
{{- end }}
{{- define "gameservice.labels" -}}
app.kubernetes.io/name: gameservice
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
