package v1beta1

import (
	"context"

	importjob "github.com/harvester/vm-import-controller/pkg/apis/importjob.harvesterhci.io/v1beta1"
	corev1 "k8s.io/api/core/v1"
	kubevirt "kubevirt.io/api/core/v1"
)

type SourceInterface interface {
	ClusterStatus() ClusterStatus
	SecretReference() corev1.SecretReference
	GenerateClient(ctx context.Context, secret *corev1.Secret) (VirtualMachineOperations, error)
}

type VirtualMachineOperations interface {
	// ExportVirtualMachine is responsible for generating the raw images for each disk associated with the VirtualMachine
	// Any image format conversion will be performed by the VM Operation
	ExportVirtualMachine(vm *importjob.VirtualMachine) error

	// PowerOffVirtualMachine is responsible for the powering off the virtualmachine
	PowerOffVirtualMachine(vm *importjob.VirtualMachine) error

	// IsPoweredOff will check the status of VM Power and return true if machine is powered off
	IsPoweredOff(vm *importjob.VirtualMachine) (bool, error)

	GenerateVirtualMachine(vm *importjob.VirtualMachine) (*kubevirt.VirtualMachine, error)
}
