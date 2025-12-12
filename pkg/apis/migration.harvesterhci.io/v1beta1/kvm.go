package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KVMSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KVMSourceSpec   `json:"spec"`
	Status            KVMSourceStatus `json:"status,omitempty"`
}

type KVMSourceSpec struct {
	LibvirtURI            string                 `json:"libvirtURI"`
	Credentials           corev1.SecretReference `json:"credentials"`
	InsecureSkipTLSVerify bool                   `json:"insecureSkipTLSVerify,omitempty"`
}

type KVMSourceStatus struct {
	Status ClusterStatus `json:"status,omitempty"`
	// +optional
	Conditions []common.Condition `json:"conditions,omitempty"`
}

func (s *KVMSource) NamespacedName() string {
	return types.NamespacedName{
		Namespace: s.Namespace,
		Name:      s.Name,
	}.String()
}

func (s *KVMSource) ClusterStatus() ClusterStatus {
	return s.Status.Status
}

func (s *KVMSource) HasSecret() bool {
	return true
}

func (s *KVMSource) SecretReference() *corev1.SecretReference {
	return &s.Spec.Credentials
}

func (s *KVMSource) GetKind() string {
	return KindKVMSource
}

func (s *KVMSource) GetConnectionInfo() (string, string) {
	return s.Spec.LibvirtURI, ""
}

type KVMSourceOptions struct {
	InsecureSkipTLSVerify bool
}

func (s *KVMSource) GetOptions() interface{} {
	return KVMSourceOptions{
		InsecureSkipTLSVerify: s.Spec.InsecureSkipTLSVerify,
	}
}
