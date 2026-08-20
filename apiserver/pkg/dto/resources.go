package dto

import (
	"encoding/json"
	"time"
)

type LabelSelector struct {
	MatchLabels      map[string]string          `json:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}
type LabelSelectorRequirement struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}
type JobAttributes struct {
	Completions                *int32 `json:"completions,omitempty"`
	Parallelism                *int32 `json:"parallelism,omitempty"`
	BackoffLimit               *int32 `json:"backoffLimit,omitempty"`
	TTLSecondsAfterFinished    *int32 `json:"ttlSecondsAfterFinished,omitempty"`
	SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty"`
	FailedJobsHistoryLimit     *int32 `json:"failedJobsHistoryLimit,omitempty"`
}
type Toleration struct {
	Key               string `json:"key,omitempty"`
	Operator          string `json:"operator,omitempty"`
	Value             string `json:"value,omitempty"`
	Effect            string `json:"effect,omitempty"`
	TolerationSeconds *int64 `json:"tolerationSeconds,omitempty"`
}
type PolicyOverride struct {
	ClusterSelector *LabelSelector        `json:"clusterSelector,omitempty"`
	Image           string                `json:"image,omitempty"`
	Env             []EnvironmentVariable `json:"env,omitempty"`
	Resources       json.RawMessage       `json:"resources,omitempty"`
	Command         []string              `json:"command,omitempty"`
	Args            []string              `json:"args,omitempty"`
}
type ResiliencyPolicy struct {
	MinAvailable   json.RawMessage `json:"minAvailable,omitempty"`
	MaxUnavailable json.RawMessage `json:"maxUnavailable,omitempty"`
}

type WorkloadRef struct {
	APIVersion string `json:"apiVersion" binding:"required"`
	Kind       string `json:"kind" binding:"required"`
}

type EnvironmentVariable struct {
	Name      string        `json:"name" binding:"required"`
	Value     string        `json:"value,omitempty"`
	ValueFrom *SecretKeyRef `json:"valueFrom,omitempty"`
}

// SecretKeyRef is an API-owned reference. Secret values never cross or persist
// at the business API boundary.
type SecretKeyRef struct {
	SecretName string `json:"secretName" binding:"required"`
	Key        string `json:"key" binding:"required"`
	Optional   bool   `json:"optional,omitempty"`
}

type Container struct {
	Name    string                `json:"name" binding:"required"`
	Image   string                `json:"image" binding:"required"`
	Command []string              `json:"command,omitempty"`
	Args    []string              `json:"args,omitempty"`
	Env     []EnvironmentVariable `json:"env,omitempty"`
}

type PodTemplate struct {
	Labels     map[string]string `json:"labels,omitempty"`
	Containers []Container       `json:"containers" binding:"required,min=1,dive"`
}

type ApplicationDesired struct {
	Workload           WorkloadRef       `json:"workload" binding:"required"`
	Replicas           *int32            `json:"replicas,omitempty"`
	Template           PodTemplate       `json:"template" binding:"required"`
	Schedule           string            `json:"schedule,omitempty"`
	Suspend            *bool             `json:"suspend,omitempty"`
	Selector           *LabelSelector    `json:"selector,omitempty"`
	JobAttributes      *JobAttributes    `json:"jobAttributes,omitempty"`
	ClusterAffinity    json.RawMessage   `json:"clusterAffinity,omitempty"`
	ClusterTolerations []Toleration      `json:"clusterTolerations,omitempty"`
	Overrides          []PolicyOverride  `json:"overrides,omitempty"`
	Resiliency         *ResiliencyPolicy `json:"resiliency,omitempty"`
	RolloutStrategy    json.RawMessage   `json:"rolloutStrategy,omitempty"`
}

type WorkspaceDesired struct {
	Namespace           string            `json:"namespace,omitempty"`
	ClusterMatchLabels  map[string]string `json:"clusterMatchLabels,omitempty"`
	ClusterSelector     *LabelSelector    `json:"clusterSelector,omitempty"`
	ResourceConstraints json.RawMessage   `json:"resourceConstraints,omitempty"`
}

type DesiredState struct {
	Application *ApplicationDesired `json:"application,omitempty"`
	Workspace   *WorkspaceDesired   `json:"workspace,omitempty"`
}

type ClusterApplicationStatus struct {
	ClusterName   string `json:"clusterName"`
	Phase         string `json:"phase"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"readyReplicas"`
	Message       string `json:"message,omitempty"`
}

type ApplicationObserved struct {
	SchedulingPhase     string                     `json:"schedulingPhase"`
	HealthPhase         string                     `json:"healthPhase"`
	GlobalReplicas      int32                      `json:"globalReplicas"`
	GlobalReadyReplicas int32                      `json:"globalReadyReplicas"`
	Clusters            []ClusterApplicationStatus `json:"clusters,omitempty"`
}

type WorkspaceClusterError struct {
	Name               string    `json:"name"`
	Message            string    `json:"message"`
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}

type WorkspaceObserved struct {
	Ready           bool                    `json:"ready"`
	AppliedClusters []string                `json:"appliedClusters,omitempty"`
	FailedClusters  []WorkspaceClusterError `json:"failedClusters,omitempty"`
}

type ClusterObserved struct {
	ConnectionMode    string     `json:"connectionMode"`
	State             string     `json:"state"`
	KubernetesVersion string     `json:"kubernetesVersion,omitempty"`
	AgentVersion      string     `json:"agentVersion,omitempty"`
	LastKeepAliveTime *time.Time `json:"lastKeepAliveTime,omitempty"`
}

type ObservedState struct {
	Application *ApplicationObserved `json:"application,omitempty"`
	Workspace   *WorkspaceObserved   `json:"workspace,omitempty"`
	Cluster     *ClusterObserved     `json:"cluster,omitempty"`
}

type ResourceView struct {
	ID         string        `json:"id"`
	Kind       string        `json:"kind"`
	Name       string        `json:"name"`
	ParentID   string        `json:"parentId,omitempty"`
	ProjectID  string        `json:"projectId,omitempty"`
	ModuleID   string        `json:"moduleId,omitempty"`
	Desired    DesiredState  `json:"desired"`
	Observed   ObservedState `json:"observed"`
	ObservedAt *time.Time    `json:"observedAt,omitempty"`
	SyncState  string        `json:"syncState"`
	Source     string        `json:"source"`
	State      string        `json:"state,omitempty"`
}
