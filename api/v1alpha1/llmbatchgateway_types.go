package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=lbg
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type LLMBatchGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LLMBatchGatewaySpec   `json:"spec,omitempty"`
	Status LLMBatchGatewayStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type LLMBatchGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMBatchGateway `json:"items"`
}

type LLMBatchGatewaySpec struct {
	// +kubebuilder:validation:Required
	SecretRef SecretReference `json:"secretRef"`

	// +kubebuilder:validation:Enum=redis;postgresql;valkey
	// +kubebuilder:default=postgresql
	DBBackend string `json:"dbBackend,omitempty"`

	FileStorage *FileStorageSpec `json:"fileStorage,omitempty"`

	// +kubebuilder:validation:Required
	APIServer APIServerSpec `json:"apiServer"`

	// +kubebuilder:validation:Required
	Processor ProcessorSpec `json:"processor"`

	GC GCSpec `json:"gc"`

	Monitoring *MonitoringSpec   `json:"monitoring,omitempty"`
	Grafana    *GrafanaSpec      `json:"grafana,omitempty"`
	TLS        *TLSSpec          `json:"tls,omitempty"`
	HTTPRoute  *HTTPRouteSpec    `json:"httpRoute,omitempty"`
	OTEL       *OTELSpec         `json:"otel,omitempty"`
	PrometheusRule *PrometheusRuleSpec `json:"prometheusRule,omitempty"`
}

type SecretReference struct {
	// +kubebuilder:validation:Required
	Name string `json:"name"`
}

// --- File Storage ---

type FileStorageSpec struct {
	// +kubebuilder:validation:Enum=fs;s3
	// +kubebuilder:default=s3
	Type string `json:"type,omitempty"`

	S3 *S3StorageSpec `json:"s3,omitempty"`
	FS *FSStorageSpec `json:"fs,omitempty"`

	Retry *FileRetrySpec `json:"retry,omitempty"`
}

type S3StorageSpec struct {
	Region           string `json:"region,omitempty"`
	Endpoint         string `json:"endpoint,omitempty"`
	AccessKeyID      string `json:"accessKeyId,omitempty"`
	Prefix           string `json:"prefix,omitempty"`
	UsePathStyle     bool   `json:"usePathStyle,omitempty"`
	AutoCreateBucket bool   `json:"autoCreateBucket,omitempty"`
}

type FSStorageSpec struct {
	BasePath string `json:"basePath,omitempty"`
	PVCName  string `json:"pvcName,omitempty"`
}

type FileRetrySpec struct {
	// +kubebuilder:default=3
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// +kubebuilder:default="1s"
	InitialBackoff string `json:"initialBackoff,omitempty"`

	// +kubebuilder:default="10s"
	MaxBackoff string `json:"maxBackoff,omitempty"`
}

// --- API Server ---

type APIServerSpec struct {
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// +kubebuilder:validation:Required
	Image string `json:"image"`

	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	Config *APIServerConfigSpec `json:"config,omitempty"`
}

type APIServerConfigSpec struct {
	Port               string `json:"port,omitempty"`
	ObservabilityPort  string `json:"observabilityPort,omitempty"`
	ReadTimeoutSeconds int32  `json:"readTimeoutSeconds,omitempty"`
	WriteTimeoutSeconds int32 `json:"writeTimeoutSeconds,omitempty"`
	IdleTimeoutSeconds int32  `json:"idleTimeoutSeconds,omitempty"`

	BatchAPI *BatchAPIConfig `json:"batchAPI,omitempty"`
	FileAPI  *FileAPIConfig  `json:"fileAPI,omitempty"`

	EnablePprof bool `json:"enablePprof,omitempty"`

	Logging *LoggingConfig `json:"logging,omitempty"`
}

type BatchAPIConfig struct {
	EventTTLSeconds    int32    `json:"eventTTLSeconds,omitempty"`
	PassThroughHeaders []string `json:"passThroughHeaders,omitempty"`
}

type FileAPIConfig struct {
	DefaultExpirationSeconds int64 `json:"defaultExpirationSeconds,omitempty"`
	MaxSizeBytes             int64 `json:"maxSizeBytes,omitempty"`
	MaxLineCount             int64 `json:"maxLineCount,omitempty"`
}

type LoggingConfig struct {
	Verbosity int32 `json:"verbosity,omitempty"`
}

// --- Processor ---

type ProcessorSpec struct {
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// +kubebuilder:validation:Required
	Image string `json:"image"`

	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	GlobalInferenceGateway *InferenceGatewaySpec            `json:"globalInferenceGateway,omitempty"`
	ModelGateways          map[string]InferenceGatewaySpec   `json:"modelGateways,omitempty"`

	Config *ProcessorConfigSpec `json:"config,omitempty"`
}

type InferenceGatewaySpec struct {
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	RequestTimeout string `json:"requestTimeout,omitempty"`
	MaxRetries     *int32 `json:"maxRetries,omitempty"`
	InitialBackoff string `json:"initialBackoff,omitempty"`
	MaxBackoff     string `json:"maxBackoff,omitempty"`

	TLSInsecureSkipVerify bool   `json:"tlsInsecureSkipVerify,omitempty"`
	TLSCACertFile         string `json:"tlsCaCertFile,omitempty"`
	TLSClientCertFile     string `json:"tlsClientCertFile,omitempty"`
	TLSClientKeyFile      string `json:"tlsClientKeyFile,omitempty"`
}

type ProcessorConfigSpec struct {
	NumWorkers             int32  `json:"numWorkers,omitempty"`
	GlobalConcurrency      int32  `json:"globalConcurrency,omitempty"`
	PerModelMaxConcurrency int32  `json:"perModelMaxConcurrency,omitempty"`
	RecoveryMaxConcurrency int32  `json:"recoveryMaxConcurrency,omitempty"`
	InferenceObjective     string `json:"inferenceObjective,omitempty"`

	DefaultOutputExpirationSeconds int64 `json:"defaultOutputExpirationSeconds,omitempty"`
	ProgressTTLSeconds             int64 `json:"progressTTLSeconds,omitempty"`

	EnablePprof bool `json:"enablePprof,omitempty"`

	Logging *LoggingConfig `json:"logging,omitempty"`
}

// --- GC ---

type GCSpec struct {
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// +kubebuilder:default="30m"
	Interval string `json:"interval,omitempty"`

	Config *GCConfigSpec `json:"config,omitempty"`
}

type GCConfigSpec struct {
	DryRun         bool  `json:"dryRun,omitempty"`
	MaxConcurrency int32 `json:"maxConcurrency,omitempty"`

	Logging *LoggingConfig `json:"logging,omitempty"`
}

// --- Observability ---

type MonitoringSpec struct {
	Enabled bool `json:"enabled,omitempty"`
}

type GrafanaSpec struct {
	Enabled bool `json:"enabled,omitempty"`
}

type PrometheusRuleSpec struct {
	Enabled bool              `json:"enabled,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

type OTELSpec struct {
	Endpoint           string `json:"endpoint,omitempty"`
	Insecure           bool   `json:"insecure,omitempty"`
	Sampler            string `json:"sampler,omitempty"`
	SamplerArg         string `json:"samplerArg,omitempty"`
	RedisTracing       bool   `json:"redisTracing,omitempty"`
	PostgresqlTracing  bool   `json:"postgresqlTracing,omitempty"`
}

// --- TLS ---

type TLSSpec struct {
	Enabled     bool             `json:"enabled,omitempty"`
	SecretName  string           `json:"secretName,omitempty"`
	CertManager *CertManagerSpec `json:"certManager,omitempty"`
}

type CertManagerSpec struct {
	IssuerName string   `json:"issuerName,omitempty"`
	IssuerKind string   `json:"issuerKind,omitempty"`
	DNSNames   []string `json:"dnsNames,omitempty"`
}

// --- HTTPRoute ---

type HTTPRouteSpec struct {
	Enabled     bool              `json:"enabled,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	ParentRefs  []ParentReference `json:"parentRefs,omitempty"`
}

type ParentReference struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace,omitempty"`
	SectionName string `json:"sectionName,omitempty"`
}

// --- Status ---

type LLMBatchGatewayStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ComponentStatus    *ComponentStatus   `json:"componentStatus,omitempty"`
}

type ComponentStatus struct {
	APIServer *ComponentReplicaStatus `json:"apiServer,omitempty"`
	Processor *ComponentReplicaStatus `json:"processor,omitempty"`
	GC        *ComponentReplicaStatus `json:"gc,omitempty"`
}

type ComponentReplicaStatus struct {
	Replicas      int32 `json:"replicas"`
	ReadyReplicas int32 `json:"readyReplicas"`
}

func init() {
	SchemeBuilder.Register(&LLMBatchGateway{}, &LLMBatchGatewayList{})
}
