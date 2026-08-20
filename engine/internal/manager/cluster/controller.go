package cluster

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	clusterv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/storage/v1alpha1"
	"github.com/fize/kumquat/engine/pkg/constants"
	"github.com/fize/kumquat/engine/pkg/observability"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fize/kumquat/engine/internal/manager/metrics"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ClusterReconciler reconciles a ManagedCluster object
type ClusterReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	HeartbeatTimeout time.Duration
	Namespace        string
	ClientManager    *ClientManager
}

// findClusterBySecretRef searches for an existing Cluster that uses the given secret name.
func (r *ClusterReconciler) findClusterBySecretRef(ctx context.Context, secretName, excludeName string) (*clusterv1alpha1.ManagedCluster, error) {
	list := &clusterv1alpha1.ManagedClusterList{}
	if err := r.List(ctx, list); err != nil {
		return nil, err
	}
	for _, c := range list.Items {
		if c.Name == excludeName {
			continue
		}
		if c.Spec.SecretRef != nil && c.Spec.SecretRef.Name == secretName {
			copy := c
			return &copy, nil
		}
	}
	return nil, nil
}

// +kubebuilder:rbac:groups=storage.kumquat.io,resources=managedclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.kumquat.io,resources=managedclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cluster clusterv1alpha1.ManagedCluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		metrics.RemoveClusterMetrics(req.Name)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	original := cluster.DeepCopy()

	ctx, span := observability.Tracer().Start(ctx, "ClusterReconcile",
		trace.WithAttributes(
			attribute.String("cluster.name", cluster.Name),
			attribute.String("cluster.mode", string(cluster.Spec.ConnectionMode)),
		),
	)
	defer span.End()

	// Update managed cluster total count
	clusterList := &clusterv1alpha1.ManagedClusterList{}
	if err := r.List(ctx, clusterList); err == nil {
		metrics.SetManagedClusterTotal(len(clusterList.Items))
	}

	mode := cluster.Spec.ConnectionMode
	if mode == "" {
		mode = clusterv1alpha1.ClusterConnectionModeHub
	}

	if mode == clusterv1alpha1.ClusterConnectionModeHub {
		return r.reconcileHub(ctx, &cluster, original)
	}
	return r.reconcileEdge(ctx, &cluster, original)
}

func (r *ClusterReconciler) reconcileHub(ctx context.Context, cluster, original *clusterv1alpha1.ManagedCluster) (ctrl.Result, error) {
	noSecretRef := cluster.Spec.SecretRef == nil || cluster.Spec.SecretRef.Name == ""

	if cluster.Spec.APIServer == clusterv1alpha1.LocalAPIServer {
		if !noSecretRef {
			cluster.Status.State = clusterv1alpha1.ClusterRejected
			return ctrl.Result{}, r.patchStatus(ctx, cluster, original)
		}
		if cluster.Status.State != clusterv1alpha1.ClusterReady {
			if cluster.Status.ID == "" {
				cluster.Status.ID = uuid.New().String()
			}
			cluster.Status.State = clusterv1alpha1.ClusterReady
			metrics.SetClusterConnectionState(cluster.Name, true)
			return ctrl.Result{}, r.patchStatus(ctx, cluster, original)
		}
		return ctrl.Result{}, nil
	}
	if noSecretRef {
		cluster.Status.State = clusterv1alpha1.ClusterRejected
		metrics.SetClusterConnectionState(cluster.Name, false)
		return ctrl.Result{}, r.patchStatus(ctx, cluster, original)
	}

	if cluster.Status.State == "" || cluster.Status.State == clusterv1alpha1.ClusterPending {
		if existing, err := r.findClusterBySecretRef(ctx, cluster.Spec.SecretRef.Name, cluster.Name); err == nil && existing != nil {
			if existing.Status.ID == "" {
				existing.Status.ID = uuid.New().String()
			}
			existing.Status.State = clusterv1alpha1.ClusterReady
			existingOriginal := existing.DeepCopy()
			if err := r.Status().Patch(ctx, existing, client.MergeFrom(existingOriginal)); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.Delete(ctx, cluster); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		if cluster.Status.ID == "" {
			cluster.Status.ID = uuid.New().String()
		}
		cluster.Status.State = clusterv1alpha1.ClusterReady
		metrics.SetClusterConnectionState(cluster.Name, true)
		if err := r.patchStatus(ctx, cluster, original); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) reconcileEdge(ctx context.Context, cluster, original *clusterv1alpha1.ManagedCluster) (ctrl.Result, error) {
	// 1. Handle Credentials from Annotations
	if cluster.Annotations != nil {
		hasCredentials := cluster.Annotations[constants.AnnotationCredentialsToken] != "" ||
			cluster.Annotations[constants.AnnotationCredentialsCert] != "" ||
			cluster.Annotations[constants.AnnotationCredentialsCA] != ""
		if hasCredentials {
			if err := r.handleEdgeCredentials(ctx, cluster); err != nil {
				return ctrl.Result{}, err
			}
			// After handling credentials, we update the object and return to re-reconcile
			return ctrl.Result{}, nil
		}
	}

	// 2. Ensure ID exists
	if cluster.Status.ID == "" {
		cluster.Status.ID = uuid.New().String()
		if err := r.patchStatus(ctx, cluster, original); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 3. Check Heartbeat
	previousState := cluster.Status.State
	if cluster.Status.LastKeepAliveTime != nil {
		heartbeatLatency := time.Since(cluster.Status.LastKeepAliveTime.Time)
		metrics.SetHeartbeatLatency(cluster.Name, heartbeatLatency)

		if heartbeatLatency > r.HeartbeatTimeout {
			if cluster.Status.State != clusterv1alpha1.ClusterOffline {
				cluster.Status.State = clusterv1alpha1.ClusterOffline
				metrics.SetClusterConnectionState(cluster.Name, false)
				ctrl.Log.Info("Cluster went offline", "cluster", cluster.Name, "last_heartbeat", cluster.Status.LastKeepAliveTime.Time)
				observability.SpanError(ctx, fmt.Errorf("heartbeat timeout: %v", heartbeatLatency))
				if err := r.patchStatus(ctx, cluster, original); err != nil {
					return ctrl.Result{}, err
				}
			}
		} else {
			// If we have a heartbeat and state is not Ready/Offline, it might be Pending
			// But handleEdgeCredentials should have set it to Ready
			if cluster.Status.State != clusterv1alpha1.ClusterReady && cluster.Status.State != clusterv1alpha1.ClusterOffline {
				cluster.Status.State = clusterv1alpha1.ClusterReady
				metrics.SetClusterConnectionState(cluster.Name, true)
				if err := r.patchStatus(ctx, cluster, original); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}

	// 4. When cluster transitions from Offline/Pending to Ready, remove stale client cache
	// so that the next GetClient call creates a fresh connection through the new tunnel session
	if cluster.Status.State == clusterv1alpha1.ClusterReady && previousState != clusterv1alpha1.ClusterReady {
		if r.ClientManager != nil {
			r.ClientManager.RemoveClient(cluster.Name)
			ctrl.Log.Info("Removed stale client cache for cluster coming online", "cluster", cluster.Name, "previousState", previousState)
		}
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *ClusterReconciler) handleEdgeCredentials(ctx context.Context, consumed *clusterv1alpha1.ManagedCluster) error {
	consumedHash := consumed.Annotations[constants.AnnotationCredentialsHash]
	if consumedHash == "" {
		return fmt.Errorf("published credential hash is required")
	}
	secretName := fmt.Sprintf("cluster-creds-%s", consumed.Name)
	secretNamespace := r.Namespace
	if secretNamespace == "" {
		secretNamespace = constants.DefaultNamespace
	}

	var applied bool
	var apiServerURL string
	clusterKey := client.ObjectKeyFromObject(consumed)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var live clusterv1alpha1.ManagedCluster
		if err := r.Get(ctx, clusterKey, &live); err != nil {
			return err
		}
		if live.Annotations[constants.AnnotationCredentialsHash] != consumedHash {
			return nil
		}
		caDataB64 := live.Annotations[constants.AnnotationCredentialsCA]
		token := live.Annotations[constants.AnnotationCredentialsToken]
		certDataB64 := live.Annotations[constants.AnnotationCredentialsCert]
		keyDataB64 := live.Annotations[constants.AnnotationCredentialsKey]
		if caDataB64 == "" || (token == "" && (certDataB64 == "" || keyDataB64 == "")) {
			return fmt.Errorf("published credentials are incomplete")
		}
		caData, err := base64.StdEncoding.DecodeString(caDataB64)
		if err != nil {
			return fmt.Errorf("decode credential CA: %w", err)
		}
		var certData, keyData []byte
		if token == "" {
			certData, err = base64.StdEncoding.DecodeString(certDataB64)
			if err != nil {
				return fmt.Errorf("decode client certificate: %w", err)
			}
			keyData, err = base64.StdEncoding.DecodeString(keyDataB64)
			if err != nil {
				return fmt.Errorf("decode client key: %w", err)
			}
		}
		apiServerURL = live.Annotations[constants.AnnotationAPIServerURL]
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: secretNamespace}}
		if _, err := ctrl.CreateOrUpdate(ctx, r.Client, secret, func() error {
			secret.Annotations = map[string]string{constants.AnnotationCredentialsAppliedHash: consumedHash}
			secret.Data = map[string][]byte{"caData": caData}
			if token != "" {
				secret.Data["token"] = []byte(token)
			} else {
				secret.Data["certData"] = certData
				secret.Data["keyData"] = keyData
			}
			return nil
		}); err != nil {
			return err
		}
		var latest clusterv1alpha1.ManagedCluster
		if err := r.Get(ctx, clusterKey, &latest); err != nil {
			return err
		}
		if latest.Annotations[constants.AnnotationCredentialsHash] != consumedHash {
			return nil
		}
		latest.Spec.SecretRef = &corev1.LocalObjectReference{Name: secretName}
		latest.Annotations[constants.AnnotationCredentialsAppliedHash] = consumedHash
		delete(latest.Annotations, constants.AnnotationCredentialsCA)
		delete(latest.Annotations, constants.AnnotationCredentialsToken)
		delete(latest.Annotations, constants.AnnotationAPIServerURL)
		delete(latest.Annotations, constants.AnnotationCredentialsCert)
		delete(latest.Annotations, constants.AnnotationCredentialsKey)
		if err := r.Update(ctx, &latest); err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil || !applied {
		return err
	}
	if r.ClientManager != nil {
		r.ClientManager.RemoveClient(consumed.Name)
	}
	var updated clusterv1alpha1.ManagedCluster
	if err := r.Get(ctx, clusterKey, &updated); err != nil {
		return err
	}
	original := updated.DeepCopy()
	updated.Status.APIServerURL = apiServerURL
	updated.Status.State = clusterv1alpha1.ClusterReady
	metrics.SetClusterConnectionState(updated.Name, true)
	return r.Status().Patch(ctx, &updated, client.MergeFrom(original))
}

func (r *ClusterReconciler) patchStatus(ctx context.Context, cluster, original *clusterv1alpha1.ManagedCluster) error {
	return r.Status().Patch(ctx, cluster, client.MergeFrom(original))
}

func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.ManagedCluster{}).
		Complete(r)
}
