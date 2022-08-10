package v1beta1

import (
	"context"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
	"github.com/harvester/vm-import-controller/pkg/source/openstack"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Openstack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OpenstackSpec   `json:"spec"`
	Status            OpenStackStatus `json:"status,omitempty"`
}

type OpenstackSpec struct {
	EndpointAddress string                 `json:"endpoint"`
	Project         string                 `json:"dc"`
	Credentials     corev1.SecretReference `json:"credentials"`
}

type OpenStackStatus struct {
	Status ClusterStatus `json:"status,omitempty"`
	// +optional
	Conditions []common.Condition `json:"conditions,omitempty"`
}

func (o *Openstack) ClusterStatus() ClusterStatus {
	return o.Status.Status
}

func (o *Openstack) GenerateClient(ctx context.Context, secret *corev1.Secret) (VirtualMachineOperations, error) {
	return openstack.NewClient(ctx, o.Spec.EndpointAddress, o.Spec.Project, secret)
}

func (o *Openstack) SecretReference() corev1.SecretReference {
	return o.Spec.Credentials
}
