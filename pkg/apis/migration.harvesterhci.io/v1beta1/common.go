package v1beta1

import (
	"github.com/rancher/wrangler/v3/pkg/condition"
	corev1 "k8s.io/api/core/v1"
)

type ImportMode string

const (
	ClusterReady          ClusterStatus  = "clusterReady"
	ClusterNotReady       ClusterStatus  = "clusterNotReady"
	ClusterReadyCondition condition.Cond = "ClusterReady"
	ClusterErrorCondition condition.Cond = "ClusterError"
)

const (
	KindVmwareSource    string = "vmwaresource"
	KindOvaSource       string = "ovasource"
	KindOpenstackSource string = "openstacksource"
)

type ClusterStatus string

type SourceInterface interface {
	ClusterStatus() ClusterStatus
	HasSecret() bool
	SecretReference() *corev1.SecretReference
	GetKind() string

	// GetConnectionInfo returns the connection information of the Source.
	GetConnectionInfo() (string, string)

	// GetOptions returns the additional configuration options of the Source.
	GetOptions() interface{}
}
