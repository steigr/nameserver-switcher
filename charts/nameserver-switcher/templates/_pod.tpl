{{/*
Pod template spec - shared between Deployment and DaemonSet
*/}}
{{- define "nameserver-switcher.podSpec" -}}
{{- with .Values.imagePullSecrets }}
imagePullSecrets:
  {{- toYaml . | nindent 2 }}
{{- end }}
serviceAccountName: {{ include "nameserver-switcher.serviceAccountName" . }}
dnsPolicy: {{ .Values.dnsPolicy }}
securityContext:
  {{- toYaml .Values.podSecurityContext | nindent 2 }}
containers:
  - name: {{ .Chart.Name }}
    securityContext:
      {{- toYaml .Values.securityContext | nindent 6 }}
    image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
    imagePullPolicy: {{ .Values.image.pullPolicy }}
    ports:
      - name: dns-udp
        containerPort: 5353
        protocol: UDP
      - name: dns-tcp
        containerPort: 5353
        protocol: TCP
      - name: grpc
        containerPort: 5354
        protocol: TCP
      - name: http
        containerPort: 8080
        protocol: TCP
    env:
      - name: DNS_PORT
        value: "5353"
      - name: GRPC_PORT
        value: "5354"
      - name: HTTP_PORT
        value: "8080"
      {{- if .Values.resolvers.request }}
      - name: REQUEST_RESOLVER
        value: {{ .Values.resolvers.request | quote }}
      {{- end }}
      {{- if .Values.resolvers.explicit }}
      - name: EXPLICIT_RESOLVER
        value: {{ .Values.resolvers.explicit | quote }}
      {{- end }}
      {{- if .Values.resolvers.passthrough }}
      - name: PASSTHROUGH_RESOLVER
        value: {{ .Values.resolvers.passthrough | quote }}
      {{- end }}
      {{- if .Values.resolvers.noCnameResponse }}
      - name: NO_CNAME_RESPONSE_RESOLVER
        value: {{ .Values.resolvers.noCnameResponse | quote }}
      {{- end }}
      {{- if .Values.resolvers.noCnameMatch }}
      - name: NO_CNAME_MATCH_RESOLVER
        value: {{ .Values.resolvers.noCnameMatch | quote }}
      {{- end }}
      {{- with .Values.extraEnv }}
      {{- toYaml . | nindent 6 }}
      {{- end }}
    envFrom:
      - configMapRef:
          name: {{ include "nameserver-switcher.fullname" . }}-patterns
    livenessProbe:
      {{- toYaml .Values.livenessProbe | nindent 6 }}
    readinessProbe:
      {{- toYaml .Values.readinessProbe | nindent 6 }}
    resources:
      {{- toYaml .Values.resources | nindent 6 }}
    volumeMounts:
      {{- with .Values.extraVolumeMounts }}
      {{- toYaml . | nindent 6 }}
      {{- end }}
volumes:
  {{- with .Values.extraVolumes }}
  {{- toYaml . | nindent 2 }}
  {{- end }}
{{- with .Values.nodeSelector }}
nodeSelector:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .Values.tolerations }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}
