package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
)

type ClusterStatus string

const (
	ClusterReady          ClusterStatus  = "clusterReady"
	ClusterNotReady       ClusterStatus  = "clusterNotReady"
	ClusterReadyCondition condition.Cond = "ClusterReady"
	ClusterErrorCondition condition.Cond = "ClusterError"
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

func (v *VmwareSource) ClusterStatus() ClusterStatus {
	return v.Status.Status
}

func (v *VmwareSource) SecretReference() corev1.SecretReference {
	return v.Spec.Credentials
}

func (v *VmwareSource) GetKind() string {
	return "vmwaresource"
}

func (v *VmwareSource) GetConnectionInfo() (string, string) {
	return v.Spec.EndpointAddress, v.Spec.Datacenter
}

func (v *VmwareSource) GetOptions() interface{} {
	return nil
}
