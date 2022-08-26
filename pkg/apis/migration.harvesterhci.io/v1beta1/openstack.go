package v1beta1

import (
	"github.com/harvester/vm-import-controller/pkg/apis/common"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OpenstackSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OpenstackSourceSpec   `json:"spec"`
	Status            OpenstackSourceStatus `json:"status,omitempty"`
}

type OpenstackSourceSpec struct {
	EndpointAddress string                 `json:"endpoint"`
	Region          string                 `json:"region"`
	Credentials     corev1.SecretReference `json:"credentials"`
}

type OpenstackSourceStatus struct {
	Status ClusterStatus `json:"status,omitempty"`
	// +optional
	Conditions []common.Condition `json:"conditions,omitempty"`
}

func (o *OpenstackSource) ClusterStatus() ClusterStatus {
	return o.Status.Status
}

func (o *OpenstackSource) SecretReference() corev1.SecretReference {
	return o.Spec.Credentials
}

func (o *OpenstackSource) GetKind() string {
	return "openstacksource"
}

func (o *OpenstackSource) GetConnectionInfo() (string, string) {
	return o.Spec.EndpointAddress, o.Spec.Region
}
