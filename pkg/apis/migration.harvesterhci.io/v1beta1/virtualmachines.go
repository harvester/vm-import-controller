package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VirtualMachineImport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualMachineImportSpec   `json:"spec"`
	Status            VirtualMachineImportStatus `json:"status,omitempty"`
}

// VirtualMachineImportSpec is used to create kubevirt VirtualMachines by exporting VM's from migration clusters.
type VirtualMachineImportSpec struct {
	SourceCluster      corev1.ObjectReference `json:"sourceCluster"`
	VirtualMachineName string                 `json:"virtualMachineName"`
	Folder             string                 `json:"folder,omitempty"`
	Mapping            []NetworkMapping       `json:"networkMapping,omitempty"` //If empty new VirtualMachineImport will be mapped to Management Network
	StorageClass       string                 `json:"storageClass,omitempty"`
}

// VirtualMachineImportStatus tracks the status of the VirtualMachineImport export from migration and import into the Harvester cluster
type VirtualMachineImportStatus struct {
	Status            ImportStatus       `json:"importStatus,omitempty"`
	DiskImportStatus  []DiskInfo         `json:"diskImportStatus,omitempty"`
	ImportConditions  []common.Condition `json:"importConditions,omitempty"`
	NewVirtualMachine string             `json:"newVirtualMachine,omitempty"`
}

// DiskInfo contains the information about associated Disk in the Import migration.
// VM's may have multiple disks, and each disk will be represented as a DiskInfo object.
// DiskInfo is used to track the following tasks
// * disk format conversion
// * path to temp disk location
// * http route to tmp disk path, as this will be exposed as a url for VirtualMachineImage
// * virtualmachineimage created from the disk route and associated file
// * conditions to track the progress of disk conversion and virtualmachineimport progress

type DiskInfo struct {
	Name                string             `json:"diskName"`
	DiskSize            int64              `json:"diskSize"`
	DiskLocalPath       string             `json:"diskLocalPath,omitempty"`
	DiskRoute           string             `json:"diskRoute,omitempty"`
	VirtualMachineImage string             `json:"VirtualMachineImage,omitempty"`
	DiskConditions      []common.Condition `json:"diskConditions,omitempty"`
	BusType             kubevirtv1.DiskBus `json:"busType" default:"virtio"`
}

type NetworkMapping struct {
	SourceNetwork      string `json:"sourceNetwork"`
	DestinationNetwork string `json:"destinationNetwork"`
}

type ImportStatus string

const (
	SourceReady                   ImportStatus   = "sourceReady"
	DisksExported                 ImportStatus   = "disksExported"
	DiskImagesSubmitted           ImportStatus   = "diskImageSubmitted"
	DiskImagesReady               ImportStatus   = "diskImagesReady"
	DiskImagesFailed              ImportStatus   = "diskImageFailed"
	VirtualMachineCreated         ImportStatus   = "virtualMachineCreated"
	VirtualMachineRunning         ImportStatus   = "virtualMachineRunning"
	VirtualMachineInvalid         ImportStatus   = "virtualMachineInvalid"
	VirtualMachinePoweringOff     condition.Cond = "VMPoweringOff"
	VirtualMachinePoweredOff      condition.Cond = "VMPoweredOff"
	VirtualMachineExported        condition.Cond = "VMExported"
	VirtualMachineImageSubmitted  condition.Cond = "VirtualMachineImageSubmitted"
	VirtualMachineImageReady      condition.Cond = "VirtualMachineImageReady"
	VirtualMachineImageFailed     condition.Cond = "VirtualMachineImageFailed"
	NotValidDNS1123Label          string         = "not a valid DNS1123 label"
	VirtualMachineExportFailed    condition.Cond = "VMExportFailed"
	VirtualMachineMigrationFailed ImportStatus   = "VMMigrationFailed"
)
