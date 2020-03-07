/*
Copyright 2019 Banzai Cloud.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pilot

import (
	"encoding/json"
	"strings"

	"github.com/ghodss/yaml"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/banzaicloud/istio-operator/pkg/apis/istio/v1beta1"
	"github.com/banzaicloud/istio-operator/pkg/resources/gateways"
	"github.com/banzaicloud/istio-operator/pkg/resources/templates"
	"github.com/banzaicloud/istio-operator/pkg/util"
)

func (r *Reconciler) configMapInjector() runtime.Object {
	return &apiv1.ConfigMap{
		ObjectMeta: templates.ObjectMeta(configMapNameInjector, sidecarInjectorLabels, r.Config),
		Data: map[string]string{
			"config": r.siConfig(),
			"values": r.getValues(),
		},
	}
}

func (r *Reconciler) getValues() string {
	podDNSSearchNamespaces := make([]string, 0)
	if util.PointerToBool(r.Config.Spec.MultiMesh) {
		podDNSSearchNamespaces = append(podDNSSearchNamespaces, []string{
			"global",
			"{{ valueOrDefault .DeploymentMeta.Namespace \"default\" }}.global",
		}...)
	}

	var zipkinTLSSettingsJSON []byte
	if r.Config.Spec.Tracing.Tracer == v1beta1.TracerTypeZipkin && r.Config.Spec.Tracing.Zipkin.TLSSettings != nil {
		zipkinTLSSettingsJSON, _ = json.Marshal(r.Config.Spec.Tracing.Zipkin.TLSSettings)
	}

	values := map[string]interface{}{
		"sidecarInjectorWebhook": map[string]interface{}{
			"rewriteAppHTTPProbe": r.Config.Spec.SidecarInjector.RewriteAppHTTPProbe,
		},
		"global": map[string]interface{}{
			"mtls": map[string]interface{}{
				"auto": util.PointerToBool(r.Config.Spec.AutoMTLS),
			},
			"istiod": map[string]interface{}{
				"enabled": util.PointerToBool(r.Config.Spec.Istiod.Enabled),
			},
			"jwtPolicy":              r.Config.Spec.JWTPolicy,
			"pilotCertProvider":      r.Config.Spec.PilotCertProvider,
			"trustDomain":            r.Config.Spec.TrustDomain,
			"imagePullPolicy":        r.Config.Spec.ImagePullPolicy,
			"network":                r.Config.Spec.NetworkName,
			"podDNSSearchNamespaces": podDNSSearchNamespaces,
			"proxy_init": map[string]interface{}{
				"image": r.Config.Spec.ProxyInit.Image,
			},
			"tracer": map[string]interface{}{
				"lightstep": map[string]interface{}{
					"CACertPath": r.Config.Spec.Tracing.Lightstep.CacertPath,
				},
			},
			"sds": map[string]interface{}{
				"customTokenDirectory": r.Config.Spec.SDS.CustomTokenDirectory,
				"enabled":              r.Config.Spec.SDS.Enabled,
				"token": map[string]interface{}{
					"aud": r.Config.Spec.SDS.TokenAudience,
				},
			},
			"multicluster": map[string]interface{}{
				"clusterName": r.Config.Spec.ClusterName,
			},
			"meshID": r.Config.Spec.MeshID,
			"proxy": map[string]interface{}{
				"image":                        r.Config.Spec.Proxy.Image,
				"statusPort":                   15020,
				"tracer":                       r.Config.Spec.Tracing.Tracer,
				"clusterDomain":                r.Config.Spec.Proxy.ClusterDomain,
				"logLevel":                     r.Config.Spec.Proxy.LogLevel,
				"componentLogLevel":            r.Config.Spec.Proxy.ComponentLogLevel,
				"dnsRefreshRate":               r.Config.Spec.Proxy.DNSRefreshRate,
				"enableCoreDump":               r.Config.Spec.Proxy.EnableCoreDump,
				"includeIPRanges":              r.Config.Spec.IncludeIPRanges,
				"excludeIPRanges":              r.Config.Spec.ExcludeIPRanges,
				"excludeInboundPorts":          "",
				"excludeOutboundPorts":         "",
				"privileged":                   r.Config.Spec.Proxy.Privileged,
				"readinessFailureThreshold":    30,
				"readinessInitialDelaySeconds": 1,
				"readinessPeriodSeconds":       2,
				"resources":                    r.Config.Spec.Proxy.Resources,
				"envoyMetricsService": map[string]interface{}{
					"enabled": r.Config.Spec.Proxy.EnvoyMetricsService.Enabled,
				},
				"envoyAccessLogService": map[string]interface{}{
					"enabled": r.Config.Spec.Proxy.EnvoyAccessLogService.Enabled,
				},
				"envoyStatsd": map[string]interface{}{
					"enabled": r.Config.Spec.Proxy.EnvoyStatsD.Enabled,
				},
				"lifecycle":             r.Config.Spec.Proxy.Lifecycle,
				"zipkinTLSSettingsJSON": string(zipkinTLSSettingsJSON),
			},
		},
	}

	j, _ := json.Marshal(&values)

	return string(j)
}

func (r *Reconciler) siConfig() string {
	autoInjection := "disabled"
	if util.PointerToBool(r.Config.Spec.SidecarInjector.AutoInjectionPolicyEnabled) {
		autoInjection = "enabled"
	}
	siConfig := map[string]interface{}{
		"policy":   autoInjection,
		"template": r.templateConfig(),
	}

	if len(r.Config.Spec.SidecarInjector.AlwaysInjectSelector) > 0 {
		siConfig["alwaysInjectSelector"] = r.Config.Spec.SidecarInjector.AlwaysInjectSelector
	}
	if len(r.Config.Spec.SidecarInjector.NeverInjectSelector) > 0 {
		siConfig["neverInjectSelector"] = r.Config.Spec.SidecarInjector.NeverInjectSelector
	}
	if len(r.Config.Spec.SidecarInjector.InjectedAnnotations) > 0 {
		siConfig["injectedAnnotations"] = r.Config.Spec.SidecarInjector.InjectedAnnotations
	}

	marshaledConfig, _ := yaml.Marshal(siConfig)
	// this is a static config, so we don't have to deal with errors
	return string(marshaledConfig)

}

func (r *Reconciler) templateConfig() string {
	return `rewriteAppHTTPProbe: {{ valueOrDefault .Values.sidecarInjectorWebhook.rewriteAppHTTPProbe false }}
{{- if .Values.global.podDNSSearchNamespaces }}
dnsConfig:
  searches:
    {{- range .Values.global.podDNSSearchNamespaces }}
    - {{ render . }}
    {{- end }}
{{- end }}
initContainers:
{{ if ne (annotation .ObjectMeta ` + "`" + `sidecar.istio.io/interceptionMode` + "`" + ` .ProxyConfig.InterceptionMode) "NONE" }}
` + r.proxyInitContainer() + `
{{ end -}}
` + r.coreDumpContainer() + `
containers:
- name: istio-proxy
  image: "{{ annotation .ObjectMeta ` + "`" + `sidecar.istio.io/proxyImage` + "`" + ` .Values.global.proxy.image }}"
  ports:
  - containerPort: 15090
    protocol: TCP
    name: http-envoy-prom
  args:
  - proxy
  - sidecar
  - --domain
  - $(POD_NAMESPACE).svc.{{ .Values.global.proxy.clusterDomain }}
  - --configPath
  - "{{ .ProxyConfig.ConfigPath }}"
  - --binaryPath
  - "{{ .ProxyConfig.BinaryPath }}"
  - --serviceCluster
  {{ if ne "" (index .ObjectMeta.Labels ` + "`" + `app` + "`" + `) -}}
  - "{{ index .ObjectMeta.Labels "app" }}.$(POD_NAMESPACE)"
  {{ else -}}
  - "{{ valueOrDefault .DeploymentMeta.Name ` + "`" + `istio-proxy` + "`" + ` }}.{{ valueOrDefault .DeploymentMeta.Namespace ` + "`" + `default` + "`" + ` }}"
  {{ end -}}
  - --drainDuration
  - "{{ formatDuration .ProxyConfig.DrainDuration }}"
  - --parentShutdownDuration
  - "{{ formatDuration .ProxyConfig.ParentShutdownDuration }}"
  - --discoveryAddress
  - "{{ annotation .ObjectMeta ` + "`" + `sidecar.istio.io/discoveryAddress` + "`" + ` .ProxyConfig.DiscoveryAddress }}"
` + r.tracingProxyArgs() + `
{{- if .Values.global.proxy.logLevel }}
  - --proxyLogLevel={{ .Values.global.proxy.logLevel }}
{{- end}}
{{- if .Values.global.proxy.componentLogLevel }}
  - --proxyComponentLogLevel={{ .Values.global.proxy.componentLogLevel }}
{{- end}}
  - --dnsRefreshRate
  - {{ .Values.global.proxy.dnsRefreshRate }}
  - --connectTimeout
  - "{{ formatDuration .ProxyConfig.ConnectTimeout }}"
  {{- if .Values.global.proxy.envoyStatsd.enabled }}
  - --statsdUdpAddress
  - "{{ .ProxyConfig.StatsdUdpAddress }}"
{{- end }}
{{- if .Values.global.proxy.envoyMetricsService.enabled }}
  - --envoyMetricsService
  - '{{ protoToJSON .ProxyConfig.EnvoyMetricsService }}'
{{- end }}
{{- if .Values.global.proxy.envoyAccessLogService.enabled }}
  - --envoyAccessLogService
  - '{{ protoToJSON .ProxyConfig.EnvoyAccessLogService }}'
{{- end }}
  - --proxyAdminPort
  - "{{ .ProxyConfig.ProxyAdminPort }}"
  {{ if gt .ProxyConfig.Concurrency 0 -}}
  - --concurrency
  - "{{ .ProxyConfig.Concurrency }}"
  {{ end -}}
  {{- if .Values.global.istiod.enabled }}
  - --controlPlaneAuthPolicy
  - NONE
  {{- else if .Values.global.controlPlaneSecurityEnabled }}
  - --controlPlaneAuthPolicy
  - MUTUAL_TLS
  {{- else }}
  - --controlPlaneAuthPolicy
  - NONE
  {{- end }}
{{- if (ne (annotation .ObjectMeta "status.sidecar.istio.io/port" (valueOrDefault .Values.global.proxy.statusPort 0 )) ` + "`" + `0` + "`" + `) }}
  - --statusPort
  - "{{ annotation .ObjectMeta ` + "`" + `status.sidecar.istio.io/port` + "`" + ` .Values.global.proxy.statusPort }}"
{{- end }}
{{- if .Values.global.trustDomain }}
  - --trust-domain={{ .Values.global.trustDomain }}
{{- end }}
  - --controlPlaneBootstrap=false
{{- if .Values.global.proxy.lifecycle }}
  lifecycle:
    {{ toYaml .Values.global.proxy.lifecycle | indent 4 }}
{{- end }}
  env:
  - name: JWT_POLICY
    value: {{ .Values.global.jwtPolicy }}
  - name: PILOT_CERT_PROVIDER
    value: {{ .Values.global.pilotCertProvider }}
  # Temp, pending PR to make it default or based on the istiodAddr env
  - name: CA_ADDR
    value: istio-pilot.istio-system.svc:15012
  - name: POD_NAME
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
  - name: POD_NAMESPACE
    valueFrom:
      fieldRef:
        fieldPath: metadata.namespace
  - name: INSTANCE_IP
    valueFrom:
      fieldRef:
        fieldPath: status.podIP
  - name: SERVICE_ACCOUNT
    valueFrom:
      fieldRef:
        fieldPath: spec.serviceAccountName
  {{- if .Values.global.mtls.auto }}
  - name: ISTIO_AUTO_MTLS_ENABLED
    value: "true"
  {{- end }}
{{- if eq .Values.global.proxy.tracer "datadog" }}
  - name: HOST_IP
    valueFrom:
      fieldRef:
        fieldPath: status.hostIP
{{- if isset .ObjectMeta.Annotations ` + "`" + `apm.datadoghq.com/env` + "`" + ` }}
{{- range $key, $value := fromJSON (index .ObjectMeta.Annotations ` + "`" + `apm.datadoghq.com/env` + "`" + `) }}
  - name: {{ $key }}
    value: "{{ $value }}"
{{- end }}
{{- end }}
{{- end }}
  - name: ISTIO_META_POD_NAME
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
  - name: ISTIO_META_CONFIG_NAMESPACE
    valueFrom:
      fieldRef:
        fieldPath: metadata.namespace
  - name: ISTIO_META_POD_PORTS
    value: |-
      [
      {{- $first := true }}
      {{- range $index1, $c := .Spec.Containers }}
        {{- range $index2, $p := $c.Ports }}
          {{- if (structToJSON $p) }}
          {{if not $first}},{{end}}{{ structToJSON $p }}
          {{- $first = false }}
          {{- end }}
        {{- end}}
      {{- end}}
      ]
  - name: ISTIO_META_CLUSTER_ID
    value: "{{ valueOrDefault .Values.global.multicluster.clusterName ` + "`" + `Kubernetes` + "`" + `}}"
{{- if eq .Values.global.proxy.tracer "zipkin" }}
  - name: ISTIO_META_ZIPKIN_TLS_SETTINGS_JSON
    value: '{{ .Values.global.proxy.zipkinTLSSettingsJSON }}'
{{- end }}
  - name: ISTIO_META_INTERCEPTION_MODE
    value: "{{ or (index .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/interceptionMode` + "`" + `) .ProxyConfig.InterceptionMode.String }}"
  {{- if .Values.global.network }}
  - name: ISTIO_META_NETWORK
    value: "{{ .Values.global.network }}"
  {{- end }}
  {{ if .ObjectMeta.Annotations }}
  - name: ISTIO_METAJSON_ANNOTATIONS
    value: |
           {{ toJSON .ObjectMeta.Annotations }}
  {{ end }}
  {{- if .DeploymentMeta.Name }}
  - name: ISTIO_META_WORKLOAD_NAME
    value: {{ .DeploymentMeta.Name }}
  {{ end }}
  {{- if and .TypeMeta.APIVersion .DeploymentMeta.Name }}
  - name: ISTIO_META_OWNER
    value: kubernetes://apis/{{ .TypeMeta.APIVersion }}/namespaces/{{ valueOrDefault .DeploymentMeta.Namespace ` + "`" + `default` + "`" + ` }}/{{ toLower .TypeMeta.Kind}}s/{{ .DeploymentMeta.Name }}
   {{- end}}
  {{- if (isset .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/bootstrapOverride` + "`" + `) }}
  - name: ISTIO_BOOTSTRAP_OVERRIDE
    value: "/etc/istio/custom-bootstrap/custom_bootstrap.json"
  {{- end }}
  {{- if .Values.global.meshID }}
  - name: ISTIO_META_MESH_ID
    value: "{{ .Values.global.meshID }}"
  {{- else if .Values.global.trustDomain }}
  - name: ISTIO_META_MESH_ID
    value: "{{ .Values.global.trustDomain }}"
  {{- end }}
  {{- if eq .Values.global.proxy.tracer "stackdriver" }}
  - name: STACKDRIVER_TRACING_ENABLED
    value: "true"
  - name: STACKDRIVER_TRACING_DEBUG
    value: "{{ .ProxyConfig.GetTracing.GetStackdriver.GetDebug }}"
  {{- if .ProxyConfig.GetTracing.GetStackdriver.GetMaxNumberOfAnnotations }}
  - name: STACKDRIVER_TRACING_MAX_NUMBER_OF_ANNOTATIONS
    value: "{{ .ProxyConfig.GetTracing.GetStackdriver.GetMaxNumberOfAnnotations }}"
  {{- end }}
  {{- if .ProxyConfig.GetTracing.GetStackdriver.GetMaxNumberOfAttributes }}
  - name: STACKDRIVER_TRACING_MAX_NUMBER_OF_ATTRIBUTES
    value: "{{ .ProxyConfig.GetTracing.GetStackdriver.GetMaxNumberOfAttributes }}"
  {{- end }}
  {{- if .ProxyConfig.GetTracing.GetStackdriver.GetMaxNumberOfMessageEvents }}
  - name: STACKDRIVER_TRACING_MAX_NUMBER_OF_MESSAGE_EVENTS
    value: "{{ .ProxyConfig.GetTracing.GetStackdriver.GetMaxNumberOfMessageEvents }}"
  {{- end }}
  {{- end }}
  imagePullPolicy: {{ .Values.global.imagePullPolicy }}
  {{ if ne (annotation .ObjectMeta ` + "`" + `status.sidecar.istio.io/port` + "`" + ` (valueOrDefault .Values.global.proxy.statusPort 0 )) ` + "`" + `0` + "`" + ` }}
  readinessProbe:
    httpGet:
      path: /healthz/ready
      port: {{ annotation .ObjectMeta ` + "`" + `status.sidecar.istio.io/port` + "`" + ` .Values.global.proxy.statusPort }}
    initialDelaySeconds: {{ annotation .ObjectMeta ` + "`" + `readiness.status.sidecar.istio.io/initialDelaySeconds` + "`" + ` .Values.global.proxy.readinessInitialDelaySeconds }}
    periodSeconds: {{ annotation .ObjectMeta ` + "`" + `readiness.status.sidecar.istio.io/periodSeconds` + "`" + ` .Values.global.proxy.readinessPeriodSeconds }}
    failureThreshold: {{ annotation .ObjectMeta ` + "`" + `readiness.status.sidecar.istio.io/failureThreshold` + "`" + ` .Values.global.proxy.readinessFailureThreshold }}
  {{ end -}}
  securityContext:
    allowPrivilegeEscalation: {{ .Values.global.proxy.privileged }}
    capabilities:
      {{ if eq (annotation .ObjectMeta ` + "`" + `sidecar.istio.io/interceptionMode` + "`" + ` .ProxyConfig.InterceptionMode) ` + "`" + `TPROXY` + "`" + ` -}}
      add:
      - NET_ADMIN
      {{- end }}
      drop:
      - ALL
    privileged: {{ .Values.global.proxy.privileged }}
    readOnlyRootFilesystem: {{ not .Values.global.proxy.enableCoreDump }}
    runAsGroup: 1337
    fsGroup: 1337
    {{ if eq (annotation .ObjectMeta ` + "`" + `sidecar.istio.io/interceptionMode` + "`" + ` .ProxyConfig.InterceptionMode) ` + "`" + `TPROXY` + "`" + ` -}}
    runAsNonRoot: false
    runAsUser: 0
    {{- else }}
    runAsNonRoot: true
    runAsUser: 1337
    {{- end }}
  resources:
  {{ if or (isset .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/proxyCPU` + "`" + `) (isset .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/proxyMemory` + "`" + `) -}}
    requests:
      {{ if (isset .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/proxyCPU` + "`" + `) -}}
      cpu: "{{ index .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/proxyCPU` + "`" + ` }}"
      {{ end}}
      {{ if (isset .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/proxyMemory` + "`" + `) -}}
      memory: "{{ index .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/proxyMemory` + "`" + ` }}"
      {{ end }}
  {{ else -}}
{{- if .Values.global.proxy.resources }}
    {{ toYaml .Values.global.proxy.resources | indent 4 }}
{{- end }}
  {{  end -}}
  volumeMounts:
  {{- if eq .Values.global.pilotCertProvider "istiod" }}
  - mountPath: /var/run/secrets/istio
    name: istiod-ca-cert
  {{- end }}
  {{ if (isset .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/bootstrapOverride` + "`" + `) }}
  - mountPath: /etc/istio/custom-bootstrap
    name: custom-bootstrap-volume
  {{- end }}
  # SDS channel between istioagent and Envoy
  - mountPath: /etc/istio/proxy
    name: istio-envoy
  {{- if eq .Values.global.jwtPolicy "third-party-jwt" }}
  - mountPath: /var/run/secrets/tokens
    name: istio-token
  {{- end }}
  {{- if .Values.global.mountMtlsCerts }}
  # Use the key and cert mounted to /etc/certs/ for the in-cluster mTLS communications.
  - mountPath: /etc/certs/
    name: istio-certs
    readOnly: true
  {{- end }}
  - name: podinfo
    mountPath: /etc/istio/pod
  {{- if and (eq .Values.global.proxy.tracer "lightstep") .Values.global.tracer.lightstep.cacertPath }}
  - mountPath: {{ directory .ProxyConfig.GetTracing.GetLightstep.GetCacertPath }}
    name: lightstep-certs
    readOnly: true
  {{- end }}
    {{- if isset .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/userVolumeMount` + "`" + ` }}
    {{ range $index, $value := fromJSON (index .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/userVolumeMount` + "`" + `) }}
  - name: "{{  $index }}"
    {{ toYaml $value | indent 4 }}
    {{ end }}
    {{- end }}
volumes:
{{- if (isset .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/bootstrapOverride` + "`" + `) }}
- name: custom-bootstrap-volume
  configMap:
    name: {{ annotation .ObjectMeta ` + "`" + `sidecar.istio.io/bootstrapOverride` + "`" + ` "" }}
{{- end }}
# SDS channel between istioagent and Envoy
- emptyDir:
    medium: Memory
  name: istio-envoy
- name: podinfo
  downwardAPI:
    items:
      - path: "labels"
        fieldRef:
          fieldPath: metadata.labels
      - path: "annotations"
        fieldRef:
          fieldPath: metadata.annotations
{{- if eq .Values.global.jwtPolicy "third-party-jwt" }}
- name: istio-token
  projected:
    sources:
    - serviceAccountToken:
        path: istio-token
        expirationSeconds: 43200
        audience: {{ .Values.global.sds.token.aud }}
{{- end }}
{{- if eq .Values.global.pilotCertProvider "istiod" }}
- name: istiod-ca-cert
  configMap:
    name: istio-ca-root-cert
{{- end }}
{{- if .Values.global.mountMtlsCerts }}
# Use the key and cert mounted to /etc/certs/ for the in-cluster mTLS communications.
- name: istio-certs
  secret:
    optional: true
    {{ if eq .Spec.ServiceAccountName "" }}
    secretName: istio.default
    {{ else -}}
    secretName: {{  printf "istio.%s" .Spec.ServiceAccountName }}
    {{  end -}}
{{- end }}
  {{- if isset .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/userVolume` + "`" + ` }}
  {{range $index, $value := fromJSON (index .ObjectMeta.Annotations ` + "`" + `sidecar.istio.io/userVolume` + "`" + `) }}
- name: "{{ $index }}"
  {{ toYaml $value | indent 2 }}
  {{ end }}
  {{ end }}
{{- if and (eq .Values.global.proxy.tracer "lightstep") .Values.global.tracer.lightstep.cacertPath }}
- name: lightstep-certs
  secret:
    optional: true
    secretName: lightstep.cacert
{{- end }}
podRedirectAnnot:
   sidecar.istio.io/interceptionMode: "{{ annotation .ObjectMeta ` + "`" + `sidecar.istio.io/interceptionMode` + "`" + ` .ProxyConfig.InterceptionMode }}"
   traffic.sidecar.istio.io/includeOutboundIPRanges: "{{ annotation .ObjectMeta ` + "`" + `traffic.sidecar.istio.io/includeOutboundIPRanges` + "`" + ` .Values.global.proxy.includeIPRanges }}"
   traffic.sidecar.istio.io/excludeOutboundIPRanges: "{{ annotation .ObjectMeta ` + "`" + `traffic.sidecar.istio.io/excludeOutboundIPRanges` + "`" + ` .Values.global.proxy.excludeIPRanges }}"
   traffic.sidecar.istio.io/includeInboundPorts: "{{ annotation .ObjectMeta ` + "`" + `traffic.sidecar.istio.io/includeInboundPorts` + "`" + ` (includeInboundPorts .Spec.Containers) }}"
   traffic.sidecar.istio.io/excludeInboundPorts: "{{ excludeInboundPort (annotation .ObjectMeta ` + "`" + `status.sidecar.istio.io/port` + "`" + ` .Values.global.proxy.statusPort) (annotation .ObjectMeta ` + "`" + `traffic.sidecar.istio.io/excludeInboundPorts` + "`" + ` .Values.global.proxy.excludeInboundPorts) }}"
{{ if or (isset .ObjectMeta.Annotations ` + "`" + `traffic.sidecar.istio.io/excludeOutboundPorts` + "`" + `) (ne .Values.global.proxy.excludeOutboundPorts "") }}
   traffic.sidecar.istio.io/excludeOutboundPorts: "{{ annotation .ObjectMeta ` + "`" + `traffic.sidecar.istio.io/excludeOutboundPorts` + "`" + ` .Values.global.proxy.excludeOutboundPorts }}"
{{- end }}
   traffic.sidecar.istio.io/kubevirtInterfaces: "{{ index .ObjectMeta.Annotations ` + "`" + `traffic.sidecar.istio.io/kubevirtInterfaces` + "`" + ` }}"
`
}

func (r *Reconciler) tracingProxyArgs() string {
	if !util.PointerToBool(r.Config.Spec.Tracing.Enabled) {
		return ""
	}

	return `{{- if eq .Values.global.proxy.tracer "lightstep" }}
  - --lightstepAddress
  - "{{ .ProxyConfig.GetTracing.GetLightstep.GetAddress }}"
  - --lightstepAccessToken
  - "{{ .ProxyConfig.GetTracing.GetLightstep.GetAccessToken }}"
  - --lightstepSecure={{ .ProxyConfig.GetTracing.GetLightstep.GetSecure }}
  - --lightstepCacertPath
  - "{{ .ProxyConfig.GetTracing.GetLightstep.GetCacertPath }}"
{{- else if eq .Values.global.proxy.tracer "zipkin" }}
  - --zipkinAddress
  - "{{ .ProxyConfig.GetTracing.GetZipkin.GetAddress }}"
{{- else if eq .Values.global.proxy.tracer "datadog" }}
  - --datadogAgentAddress
  - "{{ .ProxyConfig.GetTracing.GetDatadog.GetAddress }}"
{{- end }}
`
}

func (r *Reconciler) coreDumpContainer() string {
	if !util.PointerToBool(r.Config.Spec.Proxy.EnableCoreDump) {
		return ""
	}

	coreDumpContainerYAML, err := yaml.Marshal([]apiv1.Container{
		gateways.GetCoreDumpContainer(r.Config),
	})
	if err != nil {
		return ""
	}

	return string(coreDumpContainerYAML)
}

func (r *Reconciler) proxyInitContainer() string {
	if util.PointerToBool(r.Config.Spec.SidecarInjector.InitCNIConfiguration.Enabled) {
		return ""
	}

	return `- name: istio-init
  image: "{{ .Values.global.proxy_init.image }}"
  command:
  - istio-iptables
  - "-p"
  - "15001"
  - "-z"
  - "15006"
  - "-u"
  - 1337
  - "-m"
  - "{{ annotation .ObjectMeta ` + "`" + `sidecar.istio.io/interceptionMode` + "`" + ` .ProxyConfig.InterceptionMode }}"
  - "-i"
  - "{{ annotation .ObjectMeta ` + "`" + `traffic.sidecar.istio.io/includeOutboundIPRanges` + "`" + ` .Values.global.proxy.includeIPRanges }}"
  - "-x"
  - "{{ annotation .ObjectMeta ` + "`" + `traffic.sidecar.istio.io/excludeOutboundIPRanges` + "`" + ` .Values.global.proxy.excludeIPRanges }}"
  - "-b"
  - "{{ annotation .ObjectMeta ` + "`" + `traffic.sidecar.istio.io/includeInboundPorts` + "` `*`" + ` }}"
  - "-d"
  - "15090,{{ excludeInboundPort (annotation .ObjectMeta ` + "`" + `status.sidecar.istio.io/port` + "`" + ` .Values.global.proxy.statusPort) (annotation .ObjectMeta ` + "`" + `traffic.sidecar.istio.io/excludeInboundPorts` + "`" + ` .Values.global.proxy.excludeInboundPorts) }}"
  {{ if or (isset .ObjectMeta.Annotations ` + "`" + `traffic.sidecar.istio.io/excludeOutboundPorts` + "`" + `) (ne (valueOrDefault .Values.global.proxy.excludeOutboundPorts "") "") -}}
  - "-o"
  - "{{ annotation .ObjectMeta ` + "`" + `traffic.sidecar.istio.io/excludeOutboundPorts` + "`" + ` .Values.global.proxy.excludeOutboundPorts }}"
  {{ end -}}
  {{ if (isset .ObjectMeta.Annotations ` + "`" + `traffic.sidecar.istio.io/kubevirtInterfaces` + "`" + `) -}}
  - "-k"
  - "{{ index .ObjectMeta.Annotations ` + "`" + `traffic.sidecar.istio.io/kubevirtInterfaces` + "`" + ` }}"
  {{ end -}}
  imagePullPolicy: "{{ valueOrDefault .Values.global.imagePullPolicy  ` + "`" + `Always ` + "`" + ` }}"
` + r.getFormattedResources(r.Config.Spec.SidecarInjector.Init.Resources, 2) + `
  securityContext:
    runAsUser: 0
    runAsGroup: 0
    runAsNonRoot: false
    allowPrivilegeEscalation: {{ .Values.global.proxy.privileged }}
    privileged: {{ .Values.global.proxy.privileged }}
    capabilities:
      add:
      - NET_ADMIN
      - NET_RAW
      drop:
      - ALL
    readOnlyRootFilesystem: false
  restartPolicy: Always
  `
}

func (r *Reconciler) getFormattedResources(resources *apiv1.ResourceRequirements, indentSize int) string {
	type Resources struct {
		Resources apiv1.ResourceRequirements `json:"resources,omitempty"`
	}

	requirements := templates.GetResourcesRequirementsOrDefault(
		resources,
		r.Config.Spec.DefaultResources,
	)

	requirementsYAML, err := yaml.Marshal(Resources{
		Resources: requirements,
	})
	if err != nil {
		return ""
	}

	return indentWithSpaces(string(requirementsYAML), indentSize)
}

func indentWithSpaces(v string, spaces int) string {
	pad := strings.Repeat(" ", spaces)
	return pad + strings.Replace(v, "\n", "\n"+pad, -1)
}
