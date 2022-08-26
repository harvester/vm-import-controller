package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
)

type SourceInterface interface {
	ClusterStatus() ClusterStatus
	SecretReference() corev1.SecretReference
	GetKind() string
	GetConnectionInfo() (string, string)
}
