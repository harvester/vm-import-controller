package v1beta1

import (
	"context"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
	"github.com/harvester/vm-import-controller/pkg/source/vmware"
	"github.com/rancher/wrangler/pkg/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

type Vmware struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VmwareClusterSpec   `json:"spec"`
	Status            VmwareClusterStatus `json:"status,omitempty"`
}

type VmwareClusterSpec struct {
	EndpointAddress string                 `json:"endpoint"`
	Datacenter      string                 `json:"dc"`
	Credentials     corev1.SecretReference `json:"credentials"`
}

type VmwareClusterStatus struct {
	Status ClusterStatus `json:"status,omitempty"`
	// +optional
	Conditions []common.Condition `json:"conditions,omitempty"`
}

func (v *Vmware) ClusterStatus() ClusterStatus {
	return v.Status.Status
}

func (v *Vmware) GenerateClient(ctx context.Context, secret *corev1.Secret) (VirtualMachineOperations, error) {
	return vmware.NewClient(ctx, v.Spec.EndpointAddress, v.Spec.Datacenter, secret)
}

func (v *Vmware) SecretReference() corev1.SecretReference {
	return v.Spec.Credentials
}
