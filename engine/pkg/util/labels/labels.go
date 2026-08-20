package labels

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	// ManagedByKey is the label key used to identify resources managed by Kumquat
	ManagedByKey = "app.kubernetes.io/managed-by"
	// ManagedByValue is the label value used to identify resources managed by Kumquat
	ManagedByValue   = "kumquat"
	ApplicationIDKey = "kumquat.io/application-id"
	WorkspaceIDKey   = "kumquat.io/workspace-id"
	ModuleIDKey      = "kumquat.io/module-id"
	ProjectIDKey     = "kumquat.io/project-id"
)

// AddManagedBy adds the managed-by label to the object
func AddManagedBy(obj metav1.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[ManagedByKey] = ManagedByValue
	obj.SetLabels(labels)
}

// CopyBusinessIdentity copies Kumquat identity and ownership labels to an
// object. Mutable ownership labels belong on resource metadata, never pod
// templates or workload selectors.
func CopyBusinessIdentity(dst metav1.Object, src metav1.Object) {
	values := dst.GetLabels()
	if values == nil {
		values = map[string]string{}
	}
	for _, key := range []string{ApplicationIDKey, WorkspaceIDKey, ModuleIDKey, ProjectIDKey} {
		if value := src.GetLabels()[key]; value != "" {
			values[key] = value
		}
	}
	values[ManagedByKey] = ManagedByValue
	dst.SetLabels(values)
}

// ImmutablePodIdentity returns the only Kumquat labels allowed to participate
// in PodTemplate identity and workload selectors.
func ImmutablePodIdentity(src metav1.Object) map[string]string {
	result := map[string]string{}
	for _, key := range []string{ApplicationIDKey, WorkspaceIDKey} {
		value := src.GetLabels()[key]
		if value == "" {
			return nil
		}
		result[key] = value
	}
	result[ManagedByKey] = ManagedByValue
	return result
}
