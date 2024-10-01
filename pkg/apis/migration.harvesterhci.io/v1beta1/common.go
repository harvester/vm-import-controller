package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
)

type SourceInterface interface {
	ClusterStatus() ClusterStatus
	SecretReference() corev1.SecretReference
	GetKind() string

	// GetConnectionInfo returns the connection information of the Source.
	GetConnectionInfo() (string, string)

	// GetOptions returns the additional configuration options of the Source.
	GetOptions() interface{}
}
