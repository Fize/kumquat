package cluster

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/fize/kumquat/engine/pkg/constants"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/storage/v1alpha1"
)

func newClusterScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = clusterv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme
}

func TestClusterReconciler_PendingToReady(t *testing.T) {
	ctx := context.Background()
	scheme := newClusterScheme(t)

	cluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
		Spec: clusterv1alpha1.ManagedClusterSpec{
			SecretRef: &corev1.LocalObjectReference{Name: "test-secret"},
		},
		Status: clusterv1alpha1.ManagedClusterStatus{State: clusterv1alpha1.ClusterPending},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).WithStatusSubresource(cluster).Build()
	r := &ClusterReconciler{Client: cl, Scheme: scheme}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cluster.Name}})
	assert.NoError(t, err)

	var got clusterv1alpha1.ManagedCluster
	err = cl.Get(ctx, types.NamespacedName{Name: cluster.Name}, &got)
	assert.NoError(t, err)
	assert.Equal(t, clusterv1alpha1.ClusterReady, got.Status.State)
	assert.NotEmpty(t, got.Status.ID)
}

func TestClusterReconciler_EdgeMode_Heartbeat(t *testing.T) {
	ctx := context.Background()
	scheme := newClusterScheme(t)

	now := metav1.Now()
	cluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "edge-cluster"},
		Spec: clusterv1alpha1.ManagedClusterSpec{
			ConnectionMode: clusterv1alpha1.ClusterConnectionModeEdge,
		},
		Status: clusterv1alpha1.ManagedClusterStatus{
			State:             clusterv1alpha1.ClusterReady,
			LastKeepAliveTime: &now,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).WithStatusSubresource(cluster).Build()
	r := &ClusterReconciler{
		Client:           cl,
		Scheme:           scheme,
		HeartbeatTimeout: 1 * time.Minute,
	}

	// 1. Recent heartbeat -> should stay Ready
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cluster.Name}})
	assert.NoError(t, err)

	var got clusterv1alpha1.ManagedCluster
	cl.Get(ctx, types.NamespacedName{Name: cluster.Name}, &got)
	assert.Equal(t, clusterv1alpha1.ClusterReady, got.Status.State)

	// 2. Old heartbeat -> should become Offline
	oldTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
	got.Status.LastKeepAliveTime = &oldTime
	cl.Status().Update(ctx, &got)

	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cluster.Name}})
	assert.NoError(t, err)

	cl.Get(ctx, types.NamespacedName{Name: cluster.Name}, &got)
	assert.Equal(t, clusterv1alpha1.ClusterOffline, got.Status.State)
}

func TestClusterReconciler_EdgeMode_Credentials(t *testing.T) {
	scheme := newClusterScheme(t)

	clusterName := "edge-cluster"
	cluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: "default",
			Annotations: map[string]string{
				constants.AnnotationCredentialsToken: "test-token",
				constants.AnnotationCredentialsCA:    base64.StdEncoding.EncodeToString([]byte("test-ca")),
				constants.AnnotationCredentialsHash:  "sha256:test",
				constants.AnnotationAPIServerURL:     "https://k8s.example.com",
			},
		},
		Spec: clusterv1alpha1.ManagedClusterSpec{
			ConnectionMode: clusterv1alpha1.ClusterConnectionModeEdge,
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster).
		WithStatusSubresource(cluster).
		Build()

	r := &ClusterReconciler{
		Client:           cl,
		Scheme:           scheme,
		HeartbeatTimeout: 1 * time.Minute,
		Namespace:        "kumquat-system",
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      clusterName,
			Namespace: "default",
		},
	}

	// 1. First reconcile: Handle credentials
	_, err := r.Reconcile(context.Background(), req)
	assert.NoError(t, err)

	// Check if secret created
	secret := &corev1.Secret{}
	err = cl.Get(context.Background(), types.NamespacedName{Name: "cluster-creds-" + clusterName, Namespace: "kumquat-system"}, secret)
	assert.NoError(t, err)
	assert.Equal(t, "test-token", string(secret.Data["token"]))

	// Check if cluster updated
	updatedCluster := &clusterv1alpha1.ManagedCluster{}
	err = cl.Get(context.Background(), req.NamespacedName, updatedCluster)
	assert.NoError(t, err)
	assert.NotNil(t, updatedCluster.Spec.SecretRef)
	assert.Equal(t, "cluster-creds-"+clusterName, updatedCluster.Spec.SecretRef.Name)
	assert.Equal(t, clusterv1alpha1.ClusterReady, updatedCluster.Status.State)
	assert.Empty(t, updatedCluster.Annotations[constants.AnnotationCredentialsToken])
}

func TestClusterReconciler_HubMode_AutoAccept(t *testing.T) {
	ctx := context.Background()
	scheme := newClusterScheme(t)

	cluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "hub-no-secret"},
		Spec: clusterv1alpha1.ManagedClusterSpec{
			ConnectionMode: clusterv1alpha1.ClusterConnectionModeHub,
			APIServer:      clusterv1alpha1.LocalAPIServer,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).WithStatusSubresource(cluster).Build()
	r := &ClusterReconciler{Client: cl, Scheme: scheme}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cluster.Name}})
	assert.NoError(t, err)

	var got clusterv1alpha1.ManagedCluster
	cl.Get(ctx, types.NamespacedName{Name: cluster.Name}, &got)
	assert.Equal(t, clusterv1alpha1.ClusterReady, got.Status.State)
	assert.NotEmpty(t, got.Status.ID, "explicit local Hub cluster should be auto-accepted with an ID")
}

func TestClusterReconciler_PersistsRotatedProjectedToken(t *testing.T) {
	ctx := context.Background()
	scheme := newClusterScheme(t)
	cluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "edge-rotate", Annotations: map[string]string{
			constants.AnnotationCredentialsToken: "token-b",
			constants.AnnotationCredentialsCA:    base64.StdEncoding.EncodeToString([]byte("ca-b")),
			constants.AnnotationAPIServerURL:     "https://edge.example.test:6443",
			constants.AnnotationCredentialsHash:  "sha256:b",
		}},
		Spec: clusterv1alpha1.ManagedClusterSpec{ConnectionMode: clusterv1alpha1.ClusterConnectionModeEdge, SecretRef: &corev1.LocalObjectReference{Name: "cluster-creds-edge-rotate"}},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cluster-creds-edge-rotate", Namespace: "kumquat-system"}, Data: map[string][]byte{"token": []byte("token-a"), "caData": []byte("ca-a")}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, secret).WithStatusSubresource(cluster).Build()
	r := &ClusterReconciler{Client: cl, Scheme: scheme, Namespace: "kumquat-system"}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cluster.Name}})
	assert.NoError(t, err)
	var gotSecret corev1.Secret
	assert.NoError(t, cl.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &gotSecret))
	assert.Equal(t, "token-b", string(gotSecret.Data["token"]))
	var got clusterv1alpha1.ManagedCluster
	assert.NoError(t, cl.Get(ctx, types.NamespacedName{Name: cluster.Name}, &got))
	assert.Equal(t, "sha256:b", got.Annotations[constants.AnnotationCredentialsHash])
	assert.Equal(t, "sha256:b", got.Annotations[constants.AnnotationCredentialsAppliedHash])
	assert.Empty(t, got.Annotations[constants.AnnotationCredentialsToken])
}

func TestHandleEdgeCredentialsStaleAThenConsumesB(t *testing.T) {
	ctx := context.Background()
	scheme := newClusterScheme(t)
	annotations := func(token, hash string) map[string]string {
		return map[string]string{
			constants.AnnotationCredentialsToken: token,
			constants.AnnotationCredentialsCA:    base64.StdEncoding.EncodeToString([]byte("ca-" + token)),
			constants.AnnotationAPIServerURL:     "https://edge.example.test:6443",
			constants.AnnotationCredentialsHash:  hash,
		}
	}
	staleA := &clusterv1alpha1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "edge-cas", Annotations: annotations("token-a", "sha256:a")}, Spec: clusterv1alpha1.ManagedClusterSpec{ConnectionMode: clusterv1alpha1.ClusterConnectionModeEdge}}
	liveB := staleA.DeepCopy()
	liveB.Annotations = annotations("token-b", "sha256:b")
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cluster-creds-edge-cas", Namespace: "kumquat-system"}, Data: map[string][]byte{"token": []byte("token-old")}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(liveB, secret).WithStatusSubresource(liveB).Build()
	r := &ClusterReconciler{Client: cl, Scheme: scheme, Namespace: "kumquat-system"}
	assert.NoError(t, r.handleEdgeCredentials(ctx, staleA))
	var afterStale corev1.Secret
	assert.NoError(t, cl.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &afterStale))
	assert.Equal(t, "token-old", string(afterStale.Data["token"]), "stale A must not overwrite the Secret")
	assert.NoError(t, r.handleEdgeCredentials(ctx, liveB))
	var finalSecret corev1.Secret
	assert.NoError(t, cl.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &finalSecret))
	assert.Equal(t, "token-b", string(finalSecret.Data["token"]))
	assert.Equal(t, "sha256:b", finalSecret.Annotations[constants.AnnotationCredentialsAppliedHash])
}

func TestHandleEdgeCredentialsIncompleteDoesNotOverwriteSecret(t *testing.T) {
	ctx := context.Background()
	scheme := newClusterScheme(t)
	cluster := &clusterv1alpha1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "edge-incomplete", Annotations: map[string]string{
		constants.AnnotationCredentialsToken: "token-b",
		constants.AnnotationCredentialsHash:  "sha256:b",
	}}, Spec: clusterv1alpha1.ManagedClusterSpec{ConnectionMode: clusterv1alpha1.ClusterConnectionModeEdge}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cluster-creds-edge-incomplete", Namespace: "kumquat-system"}, Data: map[string][]byte{"token": []byte("token-a"), "caData": []byte("ca-a")}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, secret).WithStatusSubresource(cluster).Build()
	r := &ClusterReconciler{Client: cl, Scheme: scheme, Namespace: "kumquat-system"}
	assert.ErrorContains(t, r.handleEdgeCredentials(ctx, cluster), "incomplete")
	var got corev1.Secret
	assert.NoError(t, cl.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &got))
	assert.Equal(t, "token-a", string(got.Data["token"]))
	assert.Equal(t, "ca-a", string(got.Data["caData"]))
}

func TestClusterReconciler_HubMode_MissingRemoteSecretRejected(t *testing.T) {
	ctx := context.Background()
	scheme := newClusterScheme(t)
	cluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "remote-no-secret"},
		Spec: clusterv1alpha1.ManagedClusterSpec{
			ConnectionMode: clusterv1alpha1.ClusterConnectionModeHub,
			APIServer:      "https://remote.example.test:6443",
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).WithStatusSubresource(cluster).Build()
	r := &ClusterReconciler{Client: cl, Scheme: scheme}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: cluster.Name}})
	assert.NoError(t, err)
	var got clusterv1alpha1.ManagedCluster
	assert.NoError(t, cl.Get(ctx, types.NamespacedName{Name: cluster.Name}, &got))
	assert.Equal(t, clusterv1alpha1.ClusterRejected, got.Status.State)
}

func TestClusterReconciler_DuplicateSecret(t *testing.T) {
	ctx := context.Background()
	scheme := newClusterScheme(t)

	secretName := "shared-secret"

	// Existing cluster using the secret
	existingCluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "existing-cluster"},
		Spec: clusterv1alpha1.ManagedClusterSpec{
			SecretRef: &corev1.LocalObjectReference{Name: secretName},
		},
		Status: clusterv1alpha1.ManagedClusterStatus{
			ID:    "existing-id",
			State: clusterv1alpha1.ClusterReady,
		},
	}

	// New cluster trying to use the same secret
	newCluster := &clusterv1alpha1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "new-cluster"},
		Spec: clusterv1alpha1.ManagedClusterSpec{
			SecretRef: &corev1.LocalObjectReference{Name: secretName},
		},
		Status: clusterv1alpha1.ManagedClusterStatus{State: clusterv1alpha1.ClusterPending},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingCluster, newCluster).
		WithStatusSubresource(existingCluster, newCluster).
		Build()

	r := &ClusterReconciler{Client: cl, Scheme: scheme}

	// Reconcile the new cluster
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: newCluster.Name}})
	assert.NoError(t, err)

	// The new cluster should be deleted, and the existing one should be updated (if needed)
	var gotNew clusterv1alpha1.ManagedCluster
	err = cl.Get(ctx, types.NamespacedName{Name: newCluster.Name}, &gotNew)
	assert.True(t, errors.IsNotFound(err), "new cluster should be deleted")

	var gotExisting clusterv1alpha1.ManagedCluster
	err = cl.Get(ctx, types.NamespacedName{Name: existingCluster.Name}, &gotExisting)
	assert.NoError(t, err)
	assert.Equal(t, clusterv1alpha1.ClusterReady, gotExisting.Status.State)
}
