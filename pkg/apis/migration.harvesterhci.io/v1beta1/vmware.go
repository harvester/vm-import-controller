package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VmwareSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VmwareSourceSpec   `json:"spec"`
	Status            VmwareSourceStatus `json:"status,omitempty"`
}

type VmwareSourceSpec struct {
	EndpointAddress string                 `json:"endpoint"`
	Datacenter      string                 `json:"dc"`
	Credentials     corev1.SecretReference `json:"credentials"`
}

type VmwareSourceStatus struct {
	Status ClusterStatus `json:"status,omitempty"`
	// +optional
	Conditions []common.Condition `json:"conditions,omitempty"`
}

func (s *VmwareSource) NamespacedName() string {
	return types.NamespacedName{
		Namespace: s.Namespace,
		Name:      s.Name,
	}.String()
}

func (s *VmwareSource) ClusterStatus() ClusterStatus {
	return s.Status.Status
}

func (s *VmwareSource) HasSecret() bool {
	return true
}

func (s *VmwareSource) SecretReference() *corev1.SecretReference {
	return &s.Spec.Credentials
}

func (s *VmwareSource) GetKind() string {
	return KindVmwareSource
}

func (s *VmwareSource) GetConnectionInfo() (string, string) {
	return s.Spec.EndpointAddress, s.Spec.Datacenter
}

func (s *VmwareSource) GetOptions() interface{} {
	return nil
}
