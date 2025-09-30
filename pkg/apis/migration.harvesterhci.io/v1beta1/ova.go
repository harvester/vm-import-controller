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
	DefaultHttpTimeoutSeconds = 600 // 10 minutes
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OvaSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OvaSourceSpec   `json:"spec"`
	Status            OvaSourceStatus `json:"status,omitempty"`
}

type OvaSourceSpec struct {
	Url              string `json:"url"`
	OvaSourceOptions `json:",inline"`

	// The referenced `Secret` should contain the following keys:
	// - username: (optional) The username to authenticate at the specified server.
	// - password: (optional) The password to authenticate at the specified server.
	// - ca.crt: (optional) The CA certificate to verify the identity of the specified server.
	// +optional
	Credentials *corev1.SecretReference `json:"credentials,omitempty"`
}

type OvaSourceStatus struct {
	Status ClusterStatus `json:"status,omitempty"`
	// +optional
	Conditions []common.Condition `json:"conditions,omitempty"`
}

type OvaSourceOptions struct {
	// +optional
	// The HTTP timeout limit in seconds for download requests of the OVA file.
	// The timeout includes connection time, any redirects, and reading the
	// response body. A timeout of zero means no timeout.
	// Defaults to 10 minutes.
	HttpTimeoutSeconds *int `json:"httpTimeoutSeconds,omitempty"`
}

func (s *OvaSource) NamespacedName() string {
	return types.NamespacedName{
		Namespace: s.Namespace,
		Name:      s.Name,
	}.String()
}

func (s *OvaSource) ClusterStatus() ClusterStatus {
	return s.Status.Status
}

func (s *OvaSource) HasSecret() bool {
	return s.SecretReference() != nil
}

func (s *OvaSource) SecretReference() *corev1.SecretReference {
	return s.Spec.Credentials
}

func (s *OvaSource) GetKind() string {
	return KindOvaSource
}

func (s *OvaSource) GetConnectionInfo() (string, string) {
	return s.Spec.Url, ""
}

func (s *OvaSource) GetOptions() interface{} {
	return s.Spec.OvaSourceOptions
}

// GetHttpTimeout returns the HTTP timeout duration.
func (so *OvaSourceOptions) GetHttpTimeout() time.Duration {
	return time.Duration(ptr.Deref(so.HttpTimeoutSeconds, DefaultHttpTimeoutSeconds)) * time.Second
}
