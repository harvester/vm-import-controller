package v1beta1

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
)

const (
	DefaultSSHTimeoutSeconds = 10
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
	// The libvirt connection URI to connect to the KVM host.
	// E.g., qemu+ssh://user@hostname/system
	LibvirtURI string `json:"libvirtURI"`

	// Additional options.
	KVMSourceOptions `json:",inline"`

	// The referenced `Secret` should contain the following keys:
	// - username: (optional) The username to authenticate at the specified server.
	// - password: (optional) The password to authenticate at the specified server.
	// - privateKey: (optional) The private key to authenticate at the specified server.
	// One of the authentication fields password or privateKey must be specified.
	Credentials corev1.SecretReference `json:"credentials"`
}

type KVMSourceStatus struct {
	Status ClusterStatus `json:"status,omitempty"`
	// +optional
	Conditions []common.Condition `json:"conditions,omitempty"`
}

type KVMSourceOptions struct {
	// +optional
	// Timeout is the maximum amount of time in seconds for the SSH connection
	// to establish. A timeout of zero means no timeout.
	// Defaults to 10 seconds.
	SSHTimeoutSeconds *int `json:"sshTimeoutSeconds,omitempty"`
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

func (s *KVMSource) GetOptions() interface{} {
	return s.Spec.KVMSourceOptions
}

// GetSSHTimeout returns the SSH timeout duration.
func (s *KVMSourceOptions) GetSSHTimeout() time.Duration {
	return time.Duration(ptr.Deref(s.SSHTimeoutSeconds, DefaultSSHTimeoutSeconds)) * time.Second
}
