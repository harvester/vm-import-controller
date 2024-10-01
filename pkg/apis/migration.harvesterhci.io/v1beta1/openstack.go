package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
)

const (
	OpenstackDefaultRetryCount = 30
	OpenstackDefaultRetryDelay = 10
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
	EndpointAddress        string                 `json:"endpoint"`
	Region                 string                 `json:"region"`
	Credentials            corev1.SecretReference `json:"credentials"`
	OpenstackSourceOptions `json:",inline"`
}

type OpenstackSourceStatus struct {
	Status ClusterStatus `json:"status,omitempty"`
	// +optional
	Conditions []common.Condition `json:"conditions,omitempty"`
}

type OpenstackSourceOptions struct {
	// +optional
	// The number of max. retries for uploading an image.
	UploadImageRetryCount int `json:"uploadImageRetryCount,omitempty"`
	// +optional
	// The upload retry delay in seconds.
	UploadImageRetryDelay int `json:"uploadImageRetryDelay,omitempty"`
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

// GetOptions returns the sanitized OpenstackSourceOptions. This means,
// optional values are set to their default values.
func (o *OpenstackSource) GetOptions() interface{} {
	options := o.Spec.OpenstackSourceOptions
	if options.UploadImageRetryCount <= 0 {
		options.UploadImageRetryCount = OpenstackDefaultRetryCount
	}
	if options.UploadImageRetryDelay <= 0 {
		options.UploadImageRetryDelay = OpenstackDefaultRetryDelay
	}
	return options
}
