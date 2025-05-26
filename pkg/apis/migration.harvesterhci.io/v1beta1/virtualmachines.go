package v1beta1

import (
	"github.com/rancher/wrangler/pkg/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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
	SourceCluster corev1.ObjectReference `json:"sourceCluster"`

	// VirtualMachineName is the name of the virtual machine that will be
	// imported. It contains the name or ID of the source virtual machine.
	// Note that these names may not be DNS1123 compliant and will therefore
	// be sanitized later.
	// Examples: "vm-1234", "my-VM" or "5649cac7-3871-4bb5-aab6-c72b8c18d0a2"
	VirtualMachineName string `json:"virtualMachineName"`

	Folder string `json:"folder,omitempty"`

	// If empty new VirtualMachineImport will be mapped to Management Network.
	Mapping []NetworkMapping `json:"networkMapping,omitempty"`
	// The default network interface model. This is always used when:
	// - Auto-detection fails (OpenStack source client does not have auto-detection, therefore this field is used for every network interface).
	// - No network mapping is provided and a "pod-network" is auto-created.
	// Defaults to "virtio".
	DefaultNetworkInterfaceModel *string `json:"defaultNetworkInterfaceModel,omitempty" wrangler:"type=string,options=e1000|e1000e|ne2k_pci|pcnet|rtl8139|virtio"`

	StorageClass string `json:"storageClass,omitempty"`

	// The bus type that is used for imported disks if auto-detection fails.
	// Note, the OpenStack source client does not support auto-detection,
	// therefore it always makes use of this field.
	// Defaults to "virtio".
	DefaultDiskBusType *kubevirtv1.DiskBus `json:"defaultDiskBusType,omitempty"`
}

// VirtualMachineImportStatus tracks the status of the VirtualMachineImport export from migration and import into the Harvester cluster
type VirtualMachineImportStatus struct {
	Status            ImportStatus       `json:"importStatus,omitempty"`
	DiskImportStatus  []DiskInfo         `json:"diskImportStatus,omitempty"`
	ImportConditions  []common.Condition `json:"importConditions,omitempty"`
	NewVirtualMachine string             `json:"newVirtualMachine,omitempty"`

	// ImportedVirtualMachineName is the sanitized and definite name of the
	// target virtual machine that will be created in the Harvester cluster.
	// The name is DNS1123 compliant.
	ImportedVirtualMachineName string `json:"importedVirtualMachineName,omitempty"`
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
	// Override the network interface model that is auto-detected (VMware)
	// or defaulted (OpenStack).
	NetworkInterfaceModel *string `json:"networkInterfaceModel,omitempty" wrangler:"type=string,options=e1000|e1000e|ne2k_pci|pcnet|rtl8139|virtio"`
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
	VirtualMachineImportValid     ImportStatus   = "virtualMachineImportValid"
	VirtualMachineImportInvalid   ImportStatus   = "virtualMachineImportInvalid"
	VirtualMachinePoweringOff     condition.Cond = "VMPoweringOff"
	VirtualMachinePoweredOff      condition.Cond = "VMPoweredOff"
	VirtualMachineExported        condition.Cond = "VMExported"
	VirtualMachineImageSubmitted  condition.Cond = "VirtualMachineImageSubmitted"
	VirtualMachineImageReady      condition.Cond = "VirtualMachineImageReady"
	VirtualMachineImageFailed     condition.Cond = "VirtualMachineImageFailed"
	VirtualMachineExportFailed    condition.Cond = "VMExportFailed"
	VirtualMachineMigrationFailed ImportStatus   = "VMMigrationFailed"
)

// The supported network interface models.
// This can be: e1000, e1000e, ne2k_pci, pcnet, rtl8139, virtio.
// See https://kubevirt.io/user-guide/network/interfaces_and_networks/#interfaces
const (
	NetworkInterfaceModelE1000   = "e1000"
	NetworkInterfaceModelE1000e  = "e1000e"
	NetworkInterfaceModelNe2kPci = "ne2k_pci"
	NetworkInterfaceModelPcnet   = "pcnet"
	NetworkInterfaceModelRtl8139 = "rtl8139"
	NetworkInterfaceModelVirtio  = "virtio"
)

func (in *VirtualMachineImport) GetDefaultDiskBusType() kubevirtv1.DiskBus {
	return ptr.Deref(in.Spec.DefaultDiskBusType, kubevirtv1.DiskBusVirtio)
}

func (in *VirtualMachineImport) GetDefaultNetworkInterfaceModel() string {
	return ptr.Deref(in.Spec.DefaultNetworkInterfaceModel, NetworkInterfaceModelVirtio)
}

func (in *NetworkMapping) GetNetworkInterfaceModel() string {
	return ptr.Deref(in.NetworkInterfaceModel, NetworkInterfaceModelVirtio)
}
