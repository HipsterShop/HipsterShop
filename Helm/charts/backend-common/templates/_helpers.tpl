{{- define "mongoHosts" -}}
mongodb-0.mongo-headless.{{ .Values.global.namespaces.database }}.svc.cluster.local:27017,mongodb-1.mongo-headless.{{ .Values.global.namespaces.database }}.svc.cluster.local:27017,mongodb-2.mongo-headless.{{ .Values.global.namespaces.database }}.svc.cluster.local:27017
{{- end -}}
