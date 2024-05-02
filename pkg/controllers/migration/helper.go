package migration

import (
	"reflect"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/util"
)

func evaluateDiskImportStatus(diskImportStatus []migration.DiskInfo) *migration.ImportStatus {
	ok := true
	failed := false
	var failedCount, passedCount int
	for _, d := range diskImportStatus {
		ok = util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageReady, v1.ConditionTrue) && ok
		if ok {
			passedCount++
		}
		failed = util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageFailed, v1.ConditionTrue) || failed
		if failed {
			failedCount++
		}
	}

	if len(diskImportStatus) != failedCount+passedCount {
		// if length's dont match, then we have disks with missing status. Lets ignore failures for now, and handle
		// disk failures once we have had watches triggered for all disks
		return nil
	}

	if ok {
		return &[]migration.ImportStatus{migration.DiskImagesReady}[0]
	}

	if failed {
		return &[]migration.ImportStatus{migration.DiskImagesFailed}[0]
	}

	return nil
}

func (h *virtualMachineHandler) reconcileDiskImageStatus(vm *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	orgStatus := vm.Status.DeepCopy()
	// If VM has no disks associated ignore the VM
	if len(orgStatus.DiskImportStatus) == 0 {
		logrus.Errorf("Imported VM %s in namespace %s, has no disks, being marked as invalid and will be ignored", vm.Name, vm.Namespace)
		vm.Status.Status = migration.VirtualMachineInvalid
		return h.importVM.UpdateStatus(vm)

	}

	err := h.createVirtualMachineImages(vm)
	if err != nil {
		// check if any disks have been updated. We need to save this info to eventually reconcile the VMI creation
		var newVM *migration.VirtualMachineImport
		var newErr error
		if !reflect.DeepEqual(orgStatus.DiskImportStatus, vm.Status.DiskImportStatus) {
			newVM, newErr = h.importVM.UpdateStatus(vm)
		}

		if newErr != nil {
			logrus.Errorf("error updating status for vm status %s: %v", vm.Name, newErr)
		}
		return newVM, err
	}

	ok := true
	for _, d := range vm.Status.DiskImportStatus {
		ok = util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageSubmitted, v1.ConditionTrue) && ok
	}

	if ok {
		vm.Status.Status = migration.DiskImagesSubmitted
	}
	return h.importVM.UpdateStatus(vm)
}

func (h *virtualMachineHandler) reconcileVirtualMachineStatus(vm *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	// wait for VM to be running using a watch on VM's
	ok, err := h.checkVirtualMachine(vm)
	if err != nil {
		return vm, err
	}
	if !ok {
		// VM not running, requeue and check after 5mins
		h.importVM.EnqueueAfter(vm.Namespace, vm.Name, 5*time.Minute)
		return vm, nil
	}

	vm.Status.Status = migration.VirtualMachineRunning
	return h.importVM.UpdateStatus(vm)
}

func (h *virtualMachineHandler) reconcilePreFlightChecks(vm *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	err := h.preFlightChecks(vm)
	if err != nil {
		if err.Error() != migration.NotValidDNS1123Label {
			return vm, err
		}
		logrus.Errorf("vm migration target %s in VM %s in namespace %s is not RFC 1123 compliant", vm.Spec.VirtualMachineName, vm.Name, vm.Namespace)
		vm.Status.Status = migration.VirtualMachineInvalid
	} else {
		vm.Status.Status = migration.SourceReady
	}
	return h.importVM.UpdateStatus(vm)
}

func (h *virtualMachineHandler) runVirtualMachineExport(vm *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	err := h.triggerExport(vm)
	if err != nil {
		return vm, err
	}
	if util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineExported, v1.ConditionTrue) {
		vm.Status.Status = migration.DisksExported
	}

	if util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineExportFailed, v1.ConditionTrue) {
		vm.Status.Status = migration.VirtualMachineMigrationFailed
	}

	return h.importVM.UpdateStatus(vm)
}

func (h *virtualMachineHandler) triggerResubmit(vm *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	// re-export VM and trigger re-import again
	err := h.cleanupAndResubmit(vm)
	if err != nil {
		return vm, err
	}
	vm.Status.Status = migration.SourceReady
	return h.importVM.UpdateStatus(vm)
}
