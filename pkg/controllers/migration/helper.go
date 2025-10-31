package migration

import (
	"errors"
	"reflect"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/util"
)

func evaluateDiskImportStatus(diskImportStatus []migration.DiskInfo) *migration.ImportStatus {
	ok := true
	failed := false
	var failedCount, passedCount int
	for _, d := range diskImportStatus {
		ok = util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageReady, corev1.ConditionTrue) && ok
		if ok {
			passedCount++
		}
		failed = util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageFailed, corev1.ConditionTrue) || failed
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

	// If VM has no disks associated, ignore the VM.
	if len(orgStatus.DiskImportStatus) == 0 {
		logrus.WithFields(logrus.Fields{
			"name":      vm.Name,
			"namespace": vm.Namespace,
		}).Error("The imported VM has no disks, being marked as invalid and will be ignored")

		vm.Status.Status = migration.VirtualMachineImportInvalid

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
		ok = util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageSubmitted, corev1.ConditionTrue) && ok
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
		// VM not running, requeue and check after 2mins
		h.importVM.EnqueueAfter(vm.Namespace, vm.Name, 2*time.Minute)
		return vm, nil
	}

	vm.Status.Status = migration.VirtualMachineRunning

	return h.importVM.UpdateStatus(vm)
}

func (h *virtualMachineHandler) reconcilePreFlightChecks(vm *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	err := h.preFlightChecks(vm)
	if err != nil {
		if errors.Is(err, util.ErrClusterNotReady) {
			h.importVM.EnqueueAfter(vm.Namespace, vm.Name, 5*time.Second)
			return vm, err
		}

		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.sourcecluster.kind": vm.Spec.SourceCluster.Kind,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Errorf("Failed to perform source cluster specific preflight checks: %v", err)

		// Stop the reconciling for good as the checks failed.
		vm.Status.Status = migration.VirtualMachineImportInvalid
	} else {
		vm.Status.Status = migration.VirtualMachineImportValid
	}

	return h.importVM.UpdateStatus(vm)
}

func (h *virtualMachineHandler) runVirtualMachineExport(vm *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	err := h.triggerExport(vm)
	if err != nil {
		return vm, err
	}

	if util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineExported, corev1.ConditionTrue) {
		vm.Status.Status = migration.DisksExported
	}

	if util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineExportFailed, corev1.ConditionTrue) {
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

// abortMigrationIfNecessary checks whether the migration should be aborted based on the error that was passed.
// If the migration should be aborted, the status of the VirtualMachineImport resource is updated accordingly;
// otherwise the error is simply returned.
func (h *virtualMachineHandler) abortMigrationIfNecessary(vmi *migration.VirtualMachineImport, err error) (*migration.VirtualMachineImport, error) {
	if errors.Is(err, util.ErrGenerateSourceInterface) {
		vmi.Status.Status = migration.VirtualMachineMigrationFailed
		return h.importVM.UpdateStatus(vmi)
	}

	return vmi, err
}
