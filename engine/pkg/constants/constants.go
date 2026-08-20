package constants

const (
	// Annotation keys for cluster credentials
	AnnotationCredentialsCA          = "cluster.kumquat.io/credentials-ca"
	AnnotationCredentialsToken       = "cluster.kumquat.io/credentials-token"
	AnnotationAPIServerURL           = "cluster.kumquat.io/apiserver-url"
	AnnotationCredentialsCert        = "cluster.kumquat.io/credentials-cert"
	AnnotationCredentialsKey         = "cluster.kumquat.io/credentials-key"
	AnnotationCredentialsHash        = "cluster.kumquat.io/credentials-hash"
	AnnotationCredentialsAppliedHash = "cluster.kumquat.io/credentials-applied-hash"
	LabelRegistrationSource          = "kumquat.io/registration-source"
	RegistrationSourceAgent          = "agent"

	// Defaults
	DefaultNamespace           = "kumquat-system"
	DefaultAPIServerURL        = "https://kubernetes.default.svc:443"
	DefaultKubeSystemNamespace = "kube-system"
)
