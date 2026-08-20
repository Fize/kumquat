package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fize/kumquat/apiserver/pkg/dto"
	appsv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/apps/v1alpha1"
	storagev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/storage/v1alpha1"
	workspacev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/workspace/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func selectorToEngine(in *dto.LabelSelector) *metav1.LabelSelector {
	if in == nil {
		return nil
	}
	out := &metav1.LabelSelector{MatchLabels: in.MatchLabels, MatchExpressions: make([]metav1.LabelSelectorRequirement, len(in.MatchExpressions))}
	for i := range in.MatchExpressions {
		out.MatchExpressions[i] = metav1.LabelSelectorRequirement{Key: in.MatchExpressions[i].Key, Operator: metav1.LabelSelectorOperator(in.MatchExpressions[i].Operator), Values: in.MatchExpressions[i].Values}
	}
	return out
}

func selectorFromEngine(in *metav1.LabelSelector) *dto.LabelSelector {
	if in == nil {
		return nil
	}
	out := &dto.LabelSelector{MatchLabels: in.MatchLabels, MatchExpressions: make([]dto.LabelSelectorRequirement, len(in.MatchExpressions))}
	for i := range in.MatchExpressions {
		out.MatchExpressions[i] = dto.LabelSelectorRequirement{Key: in.MatchExpressions[i].Key, Operator: string(in.MatchExpressions[i].Operator), Values: in.MatchExpressions[i].Values}
	}
	return out
}

func envToEngine(in []dto.EnvironmentVariable) []corev1.EnvVar {
	out := make([]corev1.EnvVar, len(in))
	for i, env := range in {
		out[i].Name = env.Name
		if env.ValueFrom != nil {
			ref := &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: env.ValueFrom.SecretName}, Key: env.ValueFrom.Key}
			if env.ValueFrom.Optional {
				optional := true
				ref.Optional = &optional
			}
			out[i].ValueFrom = &corev1.EnvVarSource{SecretKeyRef: ref}
		} else {
			out[i].Value = env.Value
		}
	}
	return out
}

func envFromEngine(in []corev1.EnvVar) ([]dto.EnvironmentVariable, error) {
	out := make([]dto.EnvironmentVariable, len(in))
	for i, env := range in {
		out[i].Name = env.Name
		if env.Value != "" {
			return nil, fmt.Errorf("inline environment value %q is unsupported", env.Name)
		}
		if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil || env.ValueFrom.ConfigMapKeyRef != nil || env.ValueFrom.FieldRef != nil || env.ValueFrom.ResourceFieldRef != nil {
			return nil, fmt.Errorf("environment %q must use secretKeyRef", env.Name)
		}
		ref := env.ValueFrom.SecretKeyRef
		out[i].ValueFrom = &dto.SecretKeyRef{SecretName: ref.Name, Key: ref.Key}
		if ref.Optional != nil {
			out[i].ValueFrom.Optional = *ref.Optional
		}
	}
	return out, nil
}

func podTemplateToEngine(in dto.PodTemplate) corev1.PodTemplateSpec {
	containers := make([]corev1.Container, len(in.Containers))
	for i, c := range in.Containers {
		containers[i] = corev1.Container{Name: c.Name, Image: c.Image, Command: c.Command, Args: c.Args, Env: envToEngine(c.Env)}
	}
	return corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: in.Labels}, Spec: corev1.PodSpec{Containers: containers}}
}

func podTemplateFromEngine(in corev1.PodTemplateSpec) (dto.PodTemplate, error) {
	out := dto.PodTemplate{Labels: in.Labels, Containers: make([]dto.Container, len(in.Spec.Containers))}
	for i, c := range in.Spec.Containers {
		env, err := envFromEngine(c.Env)
		if err != nil {
			return dto.PodTemplate{}, err
		}
		out.Containers[i] = dto.Container{Name: c.Name, Image: c.Image, Command: c.Command, Args: c.Args, Env: env}
	}
	if !apiequality.Semantic.DeepEqual(in, podTemplateToEngine(out)) {
		return dto.PodTemplate{}, fmt.Errorf("PodTemplate contains fields outside the versioned API capability")
	}
	return out, nil
}

func applicationSpec(in dto.ApplicationDesired) (appsv1alpha1.ApplicationSpec, error) {
	template := podTemplateToEngine(in.Template)
	raw, err := json.Marshal(template)
	if err != nil {
		return appsv1alpha1.ApplicationSpec{}, err
	}
	spec := appsv1alpha1.ApplicationSpec{Workload: appsv1alpha1.WorkloadGVK{APIVersion: in.Workload.APIVersion, Kind: in.Workload.Kind}, Selector: selectorToEngine(in.Selector), Replicas: in.Replicas, Template: runtime.RawExtension{Raw: raw}, Schedule: in.Schedule, Suspend: in.Suspend}
	if in.JobAttributes != nil {
		spec.JobAttributes = &appsv1alpha1.JobAttributes{Completions: in.JobAttributes.Completions, Parallelism: in.JobAttributes.Parallelism, BackoffLimit: in.JobAttributes.BackoffLimit, TTLSecondsAfterFinished: in.JobAttributes.TTLSecondsAfterFinished, SuccessfulJobsHistoryLimit: in.JobAttributes.SuccessfulJobsHistoryLimit, FailedJobsHistoryLimit: in.JobAttributes.FailedJobsHistoryLimit}
	}
	if len(in.ClusterAffinity) > 0 {
		spec.ClusterAffinity = &corev1.NodeAffinity{}
		if err := json.Unmarshal(in.ClusterAffinity, spec.ClusterAffinity); err != nil {
			return appsv1alpha1.ApplicationSpec{}, fmt.Errorf("clusterAffinity: %w", err)
		}
	}
	for _, tol := range in.ClusterTolerations {
		spec.ClusterTolerations = append(spec.ClusterTolerations, corev1.Toleration{Key: tol.Key, Operator: corev1.TolerationOperator(tol.Operator), Value: tol.Value, Effect: corev1.TaintEffect(tol.Effect), TolerationSeconds: tol.TolerationSeconds})
	}
	for _, override := range in.Overrides {
		item := appsv1alpha1.PolicyOverride{ClusterSelector: selectorToEngine(override.ClusterSelector), Image: override.Image, Env: envToEngine(override.Env), Command: override.Command, Args: override.Args}
		if len(override.Resources) > 0 {
			item.Resources = &corev1.ResourceRequirements{}
			if err := json.Unmarshal(override.Resources, item.Resources); err != nil {
				return appsv1alpha1.ApplicationSpec{}, fmt.Errorf("override resources: %w", err)
			}
		}
		spec.Overrides = append(spec.Overrides, item)
	}
	if in.Resiliency != nil {
		spec.Resiliency = &appsv1alpha1.ResiliencyPolicy{}
		encoded, _ := json.Marshal(in.Resiliency)
		if err := json.Unmarshal(encoded, spec.Resiliency); err != nil {
			return appsv1alpha1.ApplicationSpec{}, fmt.Errorf("resiliency: %w", err)
		}
	}
	if len(in.RolloutStrategy) > 0 {
		spec.RolloutStrategy = &appsv1alpha1.RolloutStrategy{}
		if err := json.Unmarshal(in.RolloutStrategy, spec.RolloutStrategy); err != nil {
			return appsv1alpha1.ApplicationSpec{}, fmt.Errorf("rolloutStrategy: %w", err)
		}
	}
	return spec, nil
}

func applicationDesired(in appsv1alpha1.ApplicationSpec) (dto.ApplicationDesired, error) {
	var template corev1.PodTemplateSpec
	if err := json.Unmarshal(in.Template.Raw, &template); err != nil {
		return dto.ApplicationDesired{}, err
	}
	pod, err := podTemplateFromEngine(template)
	if err != nil {
		return dto.ApplicationDesired{}, err
	}
	out := dto.ApplicationDesired{Workload: dto.WorkloadRef{APIVersion: in.Workload.APIVersion, Kind: in.Workload.Kind}, Selector: selectorFromEngine(in.Selector), Replicas: in.Replicas, Template: pod, Schedule: in.Schedule, Suspend: in.Suspend}
	if in.JobAttributes != nil {
		out.JobAttributes = &dto.JobAttributes{Completions: in.JobAttributes.Completions, Parallelism: in.JobAttributes.Parallelism, BackoffLimit: in.JobAttributes.BackoffLimit, TTLSecondsAfterFinished: in.JobAttributes.TTLSecondsAfterFinished, SuccessfulJobsHistoryLimit: in.JobAttributes.SuccessfulJobsHistoryLimit, FailedJobsHistoryLimit: in.JobAttributes.FailedJobsHistoryLimit}
	}
	if in.ClusterAffinity != nil {
		out.ClusterAffinity, _ = json.Marshal(in.ClusterAffinity)
	}
	for _, tol := range in.ClusterTolerations {
		out.ClusterTolerations = append(out.ClusterTolerations, dto.Toleration{Key: tol.Key, Operator: string(tol.Operator), Value: tol.Value, Effect: string(tol.Effect), TolerationSeconds: tol.TolerationSeconds})
	}
	for _, override := range in.Overrides {
		env, err := envFromEngine(override.Env)
		if err != nil {
			return dto.ApplicationDesired{}, err
		}
		item := dto.PolicyOverride{ClusterSelector: selectorFromEngine(override.ClusterSelector), Image: override.Image, Env: env, Command: override.Command, Args: override.Args}
		if override.Resources != nil {
			item.Resources, _ = json.Marshal(override.Resources)
		}
		out.Overrides = append(out.Overrides, item)
	}
	if in.Resiliency != nil {
		encoded, _ := json.Marshal(in.Resiliency)
		out.Resiliency = &dto.ResiliencyPolicy{}
		_ = json.Unmarshal(encoded, out.Resiliency)
	}
	if in.RolloutStrategy != nil {
		out.RolloutStrategy, _ = json.Marshal(in.RolloutStrategy)
	}
	rebuilt, err := applicationSpec(out)
	if err != nil {
		return dto.ApplicationDesired{}, err
	}
	var rebuiltTemplate corev1.PodTemplateSpec
	if err := json.Unmarshal(rebuilt.Template.Raw, &rebuiltTemplate); err != nil {
		return dto.ApplicationDesired{}, err
	}
	inNoTemplate, rebuiltNoTemplate := in, rebuilt
	inNoTemplate.Template, rebuiltNoTemplate.Template = runtime.RawExtension{}, runtime.RawExtension{}
	if !apiequality.Semantic.DeepEqual(template, rebuiltTemplate) || !apiequality.Semantic.DeepEqual(inNoTemplate, rebuiltNoTemplate) {
		return dto.ApplicationDesired{}, fmt.Errorf("ApplicationSpec contains fields outside the versioned API capability")
	}
	return out, nil
}

func workspaceSpec(in dto.WorkspaceDesired) workspacev1alpha1.WorkspaceSpec {
	selector := selectorToEngine(in.ClusterSelector)
	if selector == nil && len(in.ClusterMatchLabels) > 0 {
		selector = &metav1.LabelSelector{MatchLabels: in.ClusterMatchLabels}
	}
	spec := workspacev1alpha1.WorkspaceSpec{Name: in.Namespace, ClusterSelector: selector}
	if len(in.ResourceConstraints) > 0 {
		spec.ResourceConstraints = &workspacev1alpha1.WorkspaceConstraints{}
		_ = json.Unmarshal(in.ResourceConstraints, spec.ResourceConstraints)
	}
	return spec
}

func workspaceDesired(in workspacev1alpha1.WorkspaceSpec) dto.WorkspaceDesired {
	out := dto.WorkspaceDesired{Namespace: in.Name, ClusterSelector: selectorFromEngine(in.ClusterSelector)}
	if in.ResourceConstraints != nil {
		out.ResourceConstraints, _ = json.Marshal(in.ResourceConstraints)
	}
	return out
}

func applicationObserved(in appsv1alpha1.ApplicationStatus) dto.ApplicationObserved {
	clusters := make([]dto.ClusterApplicationStatus, len(in.ClustersStatus))
	for i, c := range in.ClustersStatus {
		clusters[i] = dto.ClusterApplicationStatus{ClusterName: c.ClusterName, Phase: string(c.Phase), Replicas: c.Replicas, ReadyReplicas: c.ReadyReplicas, Message: c.Message}
	}
	return dto.ApplicationObserved{SchedulingPhase: string(in.SchedulingPhase), HealthPhase: string(in.HealthPhase), GlobalReplicas: in.GlobalReplicas, GlobalReadyReplicas: in.GlobalReadyReplicas, Clusters: clusters}
}

func workspaceObserved(in workspacev1alpha1.WorkspaceStatus) dto.WorkspaceObserved {
	ready := false
	for _, condition := range in.Conditions {
		if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
			ready = true
		}
	}
	failed := make([]dto.WorkspaceClusterError, len(in.FailedClusters))
	for i, cluster := range in.FailedClusters {
		failed[i] = dto.WorkspaceClusterError{Name: cluster.Name, Message: cluster.Message, LastTransitionTime: cluster.LastTransitionTime.Time}
	}
	return dto.WorkspaceObserved{Ready: ready, AppliedClusters: in.AppliedClusters, FailedClusters: failed}
}

func clusterObserved(in storagev1alpha1.ManagedCluster) dto.ClusterObserved {
	var keepalive *time.Time
	if in.Status.LastKeepAliveTime != nil {
		value := in.Status.LastKeepAliveTime.Time
		keepalive = &value
	}
	return dto.ClusterObserved{ConnectionMode: string(in.Spec.ConnectionMode), State: string(in.Status.State), KubernetesVersion: in.Status.KubernetesVersion, AgentVersion: in.Status.AgentVersion, LastKeepAliveTime: keepalive}
}
