package service

import (
	"encoding/json"
	"testing"

	"github.com/fize/kumquat/api/pkg/dto"
	appsv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestApplicationAdapterExplicitAdvancedAndSecretRefRoundTrip(t *testing.T) {
	resources := json.RawMessage(`{"limits":{"cpu":"1"}}`)
	in := dto.ApplicationDesired{
		Workload:           dto.WorkloadRef{APIVersion: "apps/v1", Kind: "Deployment"},
		Selector:           &dto.LabelSelector{MatchLabels: map[string]string{"tier": "api"}},
		Template:           dto.PodTemplate{Labels: map[string]string{"app": "demo"}, Containers: []dto.Container{{Name: "app", Image: "nginx", Env: []dto.EnvironmentVariable{{Name: "MAIN_TOKEN", ValueFrom: &dto.SecretKeyRef{SecretName: "main", Key: "token"}}}}}},
		ClusterTolerations: []dto.Toleration{{Key: "dedicated", Operator: "Equal", Value: "api", Effect: "NoSchedule"}},
		Overrides:          []dto.PolicyOverride{{ClusterSelector: &dto.LabelSelector{MatchLabels: map[string]string{"region": "east"}}, Image: "nginx:2", Env: []dto.EnvironmentVariable{{Name: "OVERRIDE_TOKEN", ValueFrom: &dto.SecretKeyRef{SecretName: "override", Key: "token", Optional: true}}}, Resources: resources, Command: []string{"run"}, Args: []string{"--safe"}}},
	}
	spec, err := applicationSpec(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := applicationDesired(spec)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := applicationSpec(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuilt.Overrides) != 1 || rebuilt.Overrides[0].Env[0].ValueFrom == nil || rebuilt.Overrides[0].Env[0].ValueFrom.SecretKeyRef == nil || rebuilt.Overrides[0].Env[0].ValueFrom.SecretKeyRef.Name != "override" {
		t.Fatalf("override mapping=%#v", rebuilt.Overrides)
	}
	if out.Template.Containers[0].Env[0].ValueFrom == nil || out.Template.Containers[0].Env[0].ValueFrom.SecretName != "main" {
		t.Fatalf("main env mapping=%#v", out.Template.Containers[0].Env)
	}
	if string(out.Overrides[0].Resources) == "" {
		t.Fatal("resources were lost")
	}
}

func TestApplicationAdapterRejectsUnsupportedTemplateAndInlineOverride(t *testing.T) {
	template := corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx", VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/data"}}}}, Volumes: []corev1.Volume{{Name: "data"}}}}
	raw, _ := json.Marshal(template)
	if _, err := applicationDesired(appsv1alpha1.ApplicationSpec{Workload: appsv1alpha1.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}, Template: runtime.RawExtension{Raw: raw}}); err == nil {
		t.Fatal("unsupported template was silently truncated")
	}
	supported := corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}}}
	raw, _ = json.Marshal(supported)
	spec := appsv1alpha1.ApplicationSpec{Workload: appsv1alpha1.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}, Template: runtime.RawExtension{Raw: raw}, Overrides: []appsv1alpha1.PolicyOverride{{ClusterSelector: &metav1.LabelSelector{}, Env: []corev1.EnvVar{{Name: "TOKEN", Value: "secret"}}}}}
	if _, err := applicationDesired(spec); err == nil {
		t.Fatal("inline override was accepted")
	}
}
