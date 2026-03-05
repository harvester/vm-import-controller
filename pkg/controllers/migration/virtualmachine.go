package migration

import (
	"context"
	"fmt"
	"strings"
	"time"

	harvesterutil "github.com/harvester/harvester/pkg/util"
	capiformat "sigs.k8s.io/cluster-api/util/labels/format"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	migrationController "github.com/harvester/vm-import-controller/pkg/generated/controllers/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/harvester/vm-import-controller/pkg/source/openstack"
	"github.com/harvester/vm-import-controller/pkg/source/ova"
	"github.com/harvester/vm-import-controller/pkg/source/vmware"
	"github.com/harvester/vm-import-controller/pkg/util"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvester "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	kubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	coreControllers "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/relatedresource"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	kubevirt "kubevirt.io/api/core/v1"

	storageControllers "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
)

const (
	vmImportControllerName = "virtualmachine-import-controller"
	labelImported          = "migration.harvesterhci.io/imported"
	labelImageDisplayName  = "harvesterhci.io/imageDisplayName"
	expectedAPIVersion     = "migration.harvesterhci.io/v1beta1"
)

type VirtualMachineOperations interface {
	// SanitizeVirtualMachineImport is responsible for sanitizing the VirtualMachineImport object.
	SanitizeVirtualMachineImport(vm *migration.VirtualMachineImport) error

	// ExportVirtualMachine is responsible for generating the raw images for each disk associated with the VirtualMachineImport
	// Any image format conversion will be performed by the VM Operation
	ExportVirtualMachine(vm *migration.VirtualMachineImport) error

	// ShutdownGuest is responsible for powering off the virtual machine by shutting down the guest OS
	ShutdownGuest(vm *migration.VirtualMachineImport) error

	// PowerOff is responsible for the powering off the virtual machine
	PowerOff(vm *migration.VirtualMachineImport) error

	// IsPowerOffSupported checks if the source cluster supports powering off the VM
	IsPowerOffSupported() bool

	// IsPoweredOff will check the status of VM Power and return true if the machine is powered off
	IsPoweredOff(vm *migration.VirtualMachineImport) (bool, error)

	GenerateVirtualMachine(vm *migration.VirtualMachineImport) (*kubevirt.VirtualMachine, error)

	// PreFlightChecks checks the cluster-specific configurations.
	PreFlightChecks(vm *migration.VirtualMachineImport) error

	// Cleanup is responsible for cleaning up any temporary data.
	Cleanup(vm *migration.VirtualMachineImport) error
}

type virtualMachineHandler struct {
	ctx       context.Context
	vmware    migrationController.VmwareSourceController
	ova       migrationController.OvaSourceController
	openstack migrationController.OpenstackSourceController
	secret    coreControllers.SecretController
	importVM  migrationController.VirtualMachineImportController
	vmi       harvester.VirtualMachineImageController
	kubevirt  kubevirtv1.VirtualMachineController
	pvc       coreControllers.PersistentVolumeClaimController
	sc        storageControllers.StorageClassCache
	nadCache  ctlcniv1.NetworkAttachmentDefinitionCache
}

func RegisterVMImportController(ctx context.Context, vmware migrationController.VmwareSourceController, openstack migrationController.OpenstackSourceController, ova migrationController.OvaSourceController, secret coreControllers.SecretController, importVM migrationController.VirtualMachineImportController, vmi harvester.VirtualMachineImageController, kubevirt kubevirtv1.VirtualMachineController, pvc coreControllers.PersistentVolumeClaimController, scCache storageControllers.StorageClassCache, nadCache ctlcniv1.NetworkAttachmentDefinitionCache) {
	vmHandler := &virtualMachineHandler{
		ctx:       ctx,
		vmware:    vmware,
		openstack: openstack,
		ova:       ova,
		secret:    secret,
		importVM:  importVM,
		vmi:       vmi,
		kubevirt:  kubevirt,
		pvc:       pvc,
		sc:        scCache,
		nadCache:  nadCache,
	}

	relatedresource.Watch(ctx, "virtualmachineimage-change", vmHandler.ReconcileVMI, importVM, vmi)

	importVM.OnChange(ctx, vmImportControllerName, vmHandler.OnVirtualMachineChange)
	importVM.OnRemove(ctx, vmImportControllerName, vmHandler.OnVirtualMachineRemove)
}

func (h *virtualMachineHandler) OnVirtualMachineChange(_ string, vmi *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	if vmi == nil || vmi.DeletionTimestamp != nil {
		return nil, nil
	}

	logrusEntry := logrus.WithFields(logrus.Fields{
		"name":                    vmi.Name,
		"namespace":               vmi.Namespace,
		"spec.virtualMachineName": vmi.Spec.VirtualMachineName,
	})
	vmiCopy := vmi.DeepCopy()

	switch vmiCopy.Status.Status {
	case "":
		// run preflight checks and make vmiCopy ready for import
		logrusEntry.Info("Running preflight checks ...")
		return h.reconcilePreFlightChecks(vmiCopy)
	case migration.VirtualMachineImportValid:
		logrusEntry.Info("Sanitizing the import spec ...")
		return h.abortMigrationIfNecessary(h.sanitizeVirtualMachineImport(vmiCopy))
	case migration.SourceReady:
		// vmiCopy migration is valid and ready. trigger migration specific import
		logrusEntry.Info("Importing client disk images ...")
		return h.abortMigrationIfNecessary(h.runVirtualMachineExport(vmiCopy))
	case migration.DisksExported:
		// prepare and add routes for disks to be used for VirtualMachineImage CRD
		logrusEntry.Info("Creating VM images ...")
		return h.reconcileDiskImageStatus(vmiCopy)
	case migration.DiskImagesSubmitted:
		// check and update disk image status based on VirtualMachineImage watches
		logrusEntry.Info("Evaluating VM images ...")

		err := h.reconcileVMIStatus(vmiCopy)
		if err != nil {
			return vmiCopy, err
		}

		newStatus := evaluateDiskImportStatus(vmiCopy.Status.DiskImportStatus)
		if newStatus == nil {
			return vmiCopy, nil
		}

		vmiCopy.Status.Status = *newStatus

		return h.importVM.UpdateStatus(vmiCopy)
	case migration.DiskImagesFailed:
		logrusEntry.Error("Failed to import client disk images. Try again ...")
		return h.triggerResubmit(vmiCopy)
	case migration.DiskImagesReady:
		// create VM to use the VirtualMachineObject
		logrusEntry.Info("Creating VM instances ...")

		err := h.createVirtualMachine(vmiCopy)
		if err != nil {
			return h.abortMigrationIfNecessary(vmiCopy, err)
		}

		vmiCopy.Status.Status = migration.VirtualMachineCreated

		return h.importVM.UpdateStatus(vmiCopy)
	case migration.VirtualMachineCreated:
		// wait for VM to be running using a watch on VM's
		logrusEntry.Info("Checking VM instances ...")
		return h.reconcileVirtualMachineStatus(vmiCopy)
	case migration.VirtualMachineRunning:
		logrusEntry.Info("Tidy up objects ...")

		err := h.tidyUpObjects(vmiCopy)
		if err != nil {
			return nil, err
		}

		logrusEntry.Info("The VM was imported successfully")

		err = h.triggerCleanup(vmiCopy)
		if err != nil {
			// Log the error and then drop it to prevent reconciliation loops.
			logrusEntry.Errorf("An error occurred during cleanup: %v", err)
		}

		return nil, nil
	case migration.VirtualMachineImportInvalid:
		logrusEntry.Error("The VM import spec is invalid")
		return nil, nil
	case migration.VirtualMachineMigrationFailed:
		logrusEntry.Error("The VM import has failed")

		err := h.triggerCleanup(vmiCopy)
		if err != nil {
			// Log the error and then drop it to prevent reconciliation loops.
			logrusEntry.Errorf("An error occurred during cleanup: %v", err)
		}

		return nil, nil
	}

	return nil, nil
}

func (h *virtualMachineHandler) OnVirtualMachineRemove(_ string, vmi *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	if vmi == nil {
		return nil, nil
	}

	logrusEntry := logrus.WithFields(logrus.Fields{
		"name":                    vmi.Name,
		"namespace":               vmi.Namespace,
		"spec.virtualMachineName": vmi.Spec.VirtualMachineName,
		"spec.sourceCluster.kind": vmi.Spec.SourceCluster.Kind,
		"spec.sourceCluster.name": vmi.Spec.SourceCluster.Name,
	})

	logrusEntry.Info("Cleaning up temporary data ...")

	err := h.triggerCleanup(vmi)
	if err != nil {
		// Just log the error and do not return it to prevent a reconciliation loop.
		logrusEntry.Errorf("An error occurred during cleanup: %v", err)
	}

	return nil, nil
}

// preFlightChecks is used to validate that the associate sources and VM migration references are valid
func (h *virtualMachineHandler) preFlightChecks(vm *migration.VirtualMachineImport) error {
	if vm.Spec.SourceCluster.APIVersion != expectedAPIVersion {
		return fmt.Errorf("expected migration cluster apiversion to be '%s' but got '%s'", expectedAPIVersion, vm.Spec.SourceCluster.APIVersion)
	}

	var ss migration.SourceInterface
	var err error

	switch strings.ToLower(vm.Spec.SourceCluster.Kind) {
	case migration.KindVmwareSource, migration.KindOvaSource, migration.KindOpenstackSource:
		ss, err = h.generateSource(vm)
		if err != nil {
			return fmt.Errorf("error generating migration in preflight checks: %v", err)
		}
	default:
		return fmt.Errorf("unsupported source kind %q", vm.Spec.SourceCluster.Kind)
	}

	if ss.ClusterStatus() != migration.ClusterReady {
		logrus.WithFields(logrus.Fields{
			"kind":   ss.GetKind(),
			"name":   vm.Spec.SourceCluster.Name,
			"status": ss.ClusterStatus(),
		}).Warn("The source cluster is not ready yet")
		return util.ErrClusterNotReady
	}

	// verify the specified storage class exists. Empty storage class means default storage class.
	if vm.Spec.StorageClass != "" {
		_, err := util.GetStorageClassByName(vm.Spec.StorageClass, h.sc)
		if err != nil {
			return err
		}
	}

	// dedup source network names as the same source network name cannot appear twice
	sourceNetworkMap := make(map[string]bool)
	for _, network := range vm.Spec.Mapping {
		_, ok := sourceNetworkMap[network.SourceNetwork]
		if !ok {
			sourceNetworkMap[network.SourceNetwork] = true
			continue
		}
		return fmt.Errorf("source network %s appears multiple times in VirtualMachineImport spec", network.SourceNetwork)
	}

	// Validate the destination network configuration.
	for _, nm := range vm.Spec.Mapping {
		// The destination network supports the following format:
		// - <networkName>
		// - <namespace>/<networkName>
		// See `MultusNetwork.NetworkName` for more details.
		parts := strings.Split(nm.DestinationNetwork, "/")
		switch len(parts) {
		case 1:
			// If namespace is not specified, `VirtualMachineImport` namespace is assumed.
			parts = append([]string{vm.Namespace}, parts[0])
			fallthrough
		case 2:
			_, err := h.nadCache.Get(parts[0], parts[1])
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"name":                    vm.Name,
					"namespace":               vm.Namespace,
					"spec.sourceCluster.kind": vm.Spec.SourceCluster.Kind,
					"spec.sourceCluster.name": vm.Spec.SourceCluster.Name,
				}).Errorf("Failed to get destination network '%s/%s': %v",
					parts[0], parts[1], err)
				return err
			}
		default:
			logrus.WithFields(logrus.Fields{
				"name":                    vm.Name,
				"namespace":               vm.Namespace,
				"spec.sourceCluster.kind": vm.Spec.SourceCluster.Kind,
				"spec.sourceCluster.name": vm.Spec.SourceCluster.Name,
			}).Errorf("Invalid destination network '%s'", nm.DestinationNetwork)
			return fmt.Errorf("invalid destination network '%s'", nm.DestinationNetwork)
		}
	}

	// Validate the source network as part of the source cluster preflight
	// checks.
	vmo, err := h.generateVMO(vm)
	if err != nil {
		return fmt.Errorf("error generating VMO in preFlightChecks: %w", err)
	}

	if vm.SkipPreflightChecks() {
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.sourceCluster.kind": vm.Spec.SourceCluster.Kind,
			"spec.sourceCluster.name": vm.Spec.SourceCluster.Name,
		}).Info("Skipping preflight checks")
	} else {
		err = vmo.PreFlightChecks(vm)
		if err != nil {
			return err
		}
	}

	return nil
}

// triggerShutdownGuest triggers the shutdown of the guest OS of the source VM.
func triggerShutdownGuest(vm *migration.VirtualMachineImport, vmo VirtualMachineOperations) error {
	logrus.WithFields(logrus.Fields{
		"name":                                vm.Name,
		"namespace":                           vm.Namespace,
		"spec.virtualMachineName":             vm.Spec.VirtualMachineName,
		"spec.sourceCluster.kind":             vm.Spec.SourceCluster.Kind,
		"spec.sourceCluster.name":             vm.Spec.SourceCluster.Name,
		"spec.gracefulShutdownTimeoutSeconds": vm.GetGracefulShutdownTimeoutSeconds(),
	}).Info("Shutting down guest OS of the source VM")
	err := vmo.ShutdownGuest(vm)
	if err != nil {
		return fmt.Errorf("failed to shutdown the guest OS of the source VM: %w", err)
	}
	conds := []common.Condition{
		{
			Type:               migration.VirtualMachineShutdownGuest,
			Status:             corev1.ConditionTrue,
			LastUpdateTime:     metav1.Now().Format(time.RFC3339),
			LastTransitionTime: metav1.Now().Format(time.RFC3339),
		},
	}
	vm.Status.ImportConditions = util.MergeConditions(vm.Status.ImportConditions, conds)
	return nil
}

// triggerPowerOff triggers the power off of the source VM.
func triggerPowerOff(vm *migration.VirtualMachineImport, vmo VirtualMachineOperations) error {
	logrus.WithFields(logrus.Fields{
		"name":                    vm.Name,
		"namespace":               vm.Namespace,
		"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		"spec.sourceCluster.kind": vm.Spec.SourceCluster.Kind,
		"spec.sourceCluster.name": vm.Spec.SourceCluster.Name,
	}).Info("Powering off the source VM")
	err := vmo.PowerOff(vm)
	if err != nil {
		return fmt.Errorf("failed to power off the source VM: %w", err)
	}
	conds := []common.Condition{
		{
			Type:               migration.VirtualMachinePoweringOff,
			Status:             corev1.ConditionTrue,
			LastUpdateTime:     metav1.Now().Format(time.RFC3339),
			LastTransitionTime: metav1.Now().Format(time.RFC3339),
		},
	}
	vm.Status.ImportConditions = util.MergeConditions(vm.Status.ImportConditions, conds)
	return nil
}

func (h *virtualMachineHandler) triggerExport(vm *migration.VirtualMachineImport) error {
	vmo, err := h.generateVMO(vm)
	if err != nil {
		return fmt.Errorf("error generating VMO in triggerExport: %w", err)
	}

	// Trigger power off or shutdown guest OS of the source VM.
	if vmo.IsPowerOffSupported() && vm.GetForcePowerOff() {
		if !util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachinePoweringOff, corev1.ConditionTrue) {
			return triggerPowerOff(vm, vmo)
		}
	} else {
		if !util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineShutdownGuest, corev1.ConditionTrue) {
			return triggerShutdownGuest(vm, vmo)
		}
	}

	// Check if the source VM is powered off.
	if !util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachinePoweredOff, corev1.ConditionTrue) && (util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachinePoweringOff, corev1.ConditionTrue) ||
		util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineShutdownGuest, corev1.ConditionTrue)) {
		ok, err := vmo.IsPoweredOff(vm)
		if err != nil {
			return fmt.Errorf("failed to check if source VM is powered off: %w", err)
		}
		if ok {
			conds := []common.Condition{
				{
					Type:               migration.VirtualMachinePoweredOff,
					Status:             corev1.ConditionTrue,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
				},
			}
			vm.Status.ImportConditions = util.MergeConditions(vm.Status.ImportConditions, conds)
			return nil
		}

		// Monitor a graceful shutdown by the guest OS and force a power off
		// if the shutdown is not finished within the configured time period
		// (see "vm.Spec.GracefulShutdownTimeoutSeconds").
		// Note, the following code path only applies to VMware imports.
		// OpenStack is doing a forced power off automatically if a graceful
		// shutdown of the VM guest OS was not successful within the (in
		// OpenStack) configured time period.
		shutdownGuestCondition := util.GetCondition(vm.Status.ImportConditions, migration.VirtualMachineShutdownGuest, corev1.ConditionTrue)
		if shutdownGuestCondition != nil && vmo.IsPowerOffSupported() && !vm.GetForcePowerOff() {
			// Continue only if a forced power off has not yet been triggered.
			if !util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachinePoweringOff, corev1.ConditionTrue) {
				lastUpdateTime, err := time.Parse(time.RFC3339, shutdownGuestCondition.LastUpdateTime)
				if err != nil {
					return fmt.Errorf("failed to parse the last update time of the %s condition of %s: %w",
						shutdownGuestCondition.Type, vm.NamespacedName(), err)
				}

				gracefulShutdownTimeout := time.Duration(vm.GetGracefulShutdownTimeoutSeconds()) * time.Second

				if time.Since(lastUpdateTime) > gracefulShutdownTimeout {
					logrus.WithFields(logrus.Fields{
						"name":                                vm.Name,
						"namespace":                           vm.Namespace,
						"spec.virtualMachineName":             vm.Spec.VirtualMachineName,
						"spec.sourceCluster.kind":             vm.Spec.SourceCluster.Kind,
						"spec.sourceCluster.name":             vm.Spec.SourceCluster.Name,
						"spec.gracefulShutdownTimeoutSeconds": vm.GetGracefulShutdownTimeoutSeconds(),
					}).Info("Forcing power off of the source VM because the guest OS did not gracefully shutdown within the configured time period")
					return triggerPowerOff(vm, vmo)
				}
			}
		}

		// Trigger another reconciliation in N seconds.
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
			"spec.sourceCluster.kind": vm.Spec.SourceCluster.Kind,
			"spec.sourceCluster.name": vm.Spec.SourceCluster.Name,
		}).Info("Waiting for VM to be powered off ...")
		h.importVM.EnqueueAfter(vm.Namespace, vm.Name, 5*time.Second)

		return nil
	}

	if util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachinePoweredOff, corev1.ConditionTrue) &&
		(util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachinePoweringOff, corev1.ConditionTrue) || util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineShutdownGuest, corev1.ConditionTrue)) &&
		!util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineExported, corev1.ConditionTrue) &&
		!util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineExportFailed, corev1.ConditionTrue) {
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
			"spec.sourceCluster.name": vm.Spec.SourceCluster.Name,
			"spec.sourceCluster.kind": vm.Spec.SourceCluster.Kind,
		}).Info("Exporting source VM")
		err := vmo.ExportVirtualMachine(vm)
		if err != nil {
			// avoid retrying if vm export fails
			conds := []common.Condition{
				{
					Type:               migration.VirtualMachineExportFailed,
					Status:             corev1.ConditionTrue,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
					Message:            fmt.Sprintf("error exporting VM: %v", err),
				},
			}
			vm.Status.ImportConditions = util.MergeConditions(vm.Status.ImportConditions, conds)
			logrus.WithFields(logrus.Fields{
				"name":                    vm.Name,
				"namespace":               vm.Namespace,
				"spec.virtualMachineName": vm.Spec.VirtualMachineName,
				"spec.sourceCluster.name": vm.Spec.SourceCluster.Name,
				"spec.sourceCluster.kind": vm.Spec.SourceCluster.Kind,
			}).Errorf("Failed to export source VM: %v", err)
			return nil
		}
		conds := []common.Condition{
			{
				Type:               migration.VirtualMachineExported,
				Status:             corev1.ConditionTrue,
				LastUpdateTime:     metav1.Now().Format(time.RFC3339),
				LastTransitionTime: metav1.Now().Format(time.RFC3339),
			},
		}
		vm.Status.ImportConditions = util.MergeConditions(vm.Status.ImportConditions, conds)
		return nil
	}

	return nil
}

// generateVMO is a wrapper to generate a VirtualMachineOperations client
func (h *virtualMachineHandler) generateVMO(vm *migration.VirtualMachineImport) (VirtualMachineOperations, error) {
	source, err := h.generateSource(vm)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
			"spec.sourceCluster.kind": vm.Spec.SourceCluster.Kind,
			"spec.sourceCluster.name": vm.Spec.SourceCluster.Name,
		}).Errorf("Failed to generate source interface: %v", err)
		return nil, util.ErrGenerateSourceInterface
	}

	// Secrets can be used optionally in `Source` CRs.
	var secret *corev1.Secret
	if source.HasSecret() {
		secretRef := source.SecretReference()
		if secretRef == nil {
			return nil, fmt.Errorf("secret reference is nil")
		}
		secret, err = h.secret.Get(secretRef.Namespace, secretRef.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("error fetching secret: %w", err)
		}
	}

	// Generate `VirtualMachineOperations` interface which will be used for
	// migration-specific operations.
	switch strings.ToLower(source.GetKind()) {
	case migration.KindVmwareSource:
		endpoint, dc := source.GetConnectionInfo()
		return vmware.NewClient(h.ctx, endpoint, dc, secret)
	case migration.KindOvaSource:
		url, _ := source.GetConnectionInfo()
		options := source.GetOptions().(migration.OvaSourceOptions)
		return ova.NewClient(h.ctx, url, secret, options)
	case migration.KindOpenstackSource:
		endpoint, region := source.GetConnectionInfo()
		options := source.GetOptions().(migration.OpenstackSourceOptions)
		return openstack.NewClient(h.ctx, endpoint, region, secret, options)
	}

	return nil, fmt.Errorf("source kind %q not supported", source.GetKind())
}

func (h *virtualMachineHandler) generateSource(vm *migration.VirtualMachineImport) (migration.SourceInterface, error) {
	var si migration.SourceInterface
	var err error

	switch strings.ToLower(vm.Spec.SourceCluster.Kind) {
	case migration.KindVmwareSource:
		si, err = h.vmware.Get(vm.Spec.SourceCluster.Namespace, vm.Spec.SourceCluster.Name, metav1.GetOptions{})
	case migration.KindOvaSource:
		si, err = h.ova.Get(vm.Spec.SourceCluster.Namespace, vm.Spec.SourceCluster.Name, metav1.GetOptions{})
	case migration.KindOpenstackSource:
		si, err = h.openstack.Get(vm.Spec.SourceCluster.Namespace, vm.Spec.SourceCluster.Name, metav1.GetOptions{})
	default:
		err = fmt.Errorf("source kind %q not supported", vm.Spec.SourceCluster.Kind)
	}

	return si, err
}

func (h *virtualMachineHandler) createVirtualMachineImages(vm *migration.VirtualMachineImport) error {
	// check and create VirtualMachineImage objects
	status := vm.Status.DeepCopy()
	for i, d := range status.DiskImportStatus {
		if !util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageSubmitted, corev1.ConditionTrue) {
			vmiObj, err := h.checkAndCreateVirtualMachineImage(vm, d)
			if err != nil {
				return fmt.Errorf("error creating vmi: %v", err)
			}
			d.VirtualMachineImage = vmiObj.Name
			vm.Status.DiskImportStatus[i] = d
			cond := []common.Condition{
				{
					Type:               migration.VirtualMachineImageSubmitted,
					Status:             corev1.ConditionTrue,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
				},
			}
			vm.Status.DiskImportStatus[i].DiskConditions = util.MergeConditions(vm.Status.DiskImportStatus[i].DiskConditions, cond)
		}
	}

	return nil
}

func (h *virtualMachineHandler) reconcileVMIStatus(vm *migration.VirtualMachineImport) error {
	for i, d := range vm.Status.DiskImportStatus {
		if !util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageReady, corev1.ConditionTrue) {
			vmi, err := h.vmi.Get(vm.Namespace, d.VirtualMachineImage, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("error quering vmi in reconcileVMIStatus: %v", err)
			}
			for _, v := range vmi.Status.Conditions {
				if v.Type == harvesterv1beta1.ImageImported && v.Status == corev1.ConditionTrue {
					cond := []common.Condition{
						{
							Type:               migration.VirtualMachineImageReady,
							Status:             corev1.ConditionTrue,
							LastUpdateTime:     metav1.Now().Format(time.RFC3339),
							LastTransitionTime: metav1.Now().Format(time.RFC3339),
						},
					}
					d.DiskConditions = util.MergeConditions(d.DiskConditions, cond)
					vm.Status.DiskImportStatus[i] = d
				}

				// handle failed imports if any
				if v.Type == harvesterv1beta1.ImageImported && v.Status == corev1.ConditionFalse && v.Reason == "ImportFailed" {
					cond := []common.Condition{
						{
							Type:               migration.VirtualMachineImageFailed,
							Status:             corev1.ConditionTrue,
							LastUpdateTime:     metav1.Now().Format(time.RFC3339),
							LastTransitionTime: metav1.Now().Format(time.RFC3339),
						},
					}
					d.DiskConditions = util.MergeConditions(d.DiskConditions, cond)
					vm.Status.DiskImportStatus[i] = d
				}

			}
		}
	}
	return nil
}

func (h *virtualMachineHandler) createVirtualMachine(vm *migration.VirtualMachineImport) error {
	vmo, err := h.generateVMO(vm)
	if err != nil {
		return fmt.Errorf("error generating VMO in createVirtualMachine: %w", err)
	}

	runVM, err := vmo.GenerateVirtualMachine(vm)
	if err != nil {
		return fmt.Errorf("error generating Kubevirt VM: %v", err)
	}

	// create PVC claims from VMI's to create the Kubevirt VM
	err = h.findAndCreatePVC(vm)
	if err != nil {
		return err
	}

	// patch VM object with PVC info
	vmVols := make([]kubevirt.Volume, 0, len(vm.Status.DiskImportStatus))
	disks := make([]kubevirt.Disk, 0, len(vm.Status.DiskImportStatus))
	for i, v := range vm.Status.DiskImportStatus {
		pvcName := v.VirtualMachineImage
		vmVols = append(vmVols, kubevirt.Volume{
			Name: fmt.Sprintf("disk-%d", i),
			VolumeSource: kubevirt.VolumeSource{
				PersistentVolumeClaim: &kubevirt.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
					},
				},
			},
		})
		diskOrder := i
		diskOrder++ // Disk order cant be 0, so need to kick things off from 1
		disks = append(disks, kubevirt.Disk{
			Name:      fmt.Sprintf("disk-%d", i),
			BootOrder: &[]uint{uint(diskOrder)}[0], // nolint:gosec
			DiskDevice: kubevirt.DiskDevice{
				Disk: &kubevirt.DiskTarget{
					Bus: v.BusType,
				},
			},
		})
	}

	runVM.Spec.Template.Spec.Volumes = vmVols
	runVM.Spec.Template.Spec.Domain.Devices.Disks = disks

	// Apply a label to the `VirtualMachine` object to make the newly
	// created VM identifiable.
	metav1.SetMetaDataLabel(&runVM.ObjectMeta, labelImported, "true")

	// Make sure the new VM is created only if it does not exist.
	found := false
	existingVM, err := h.kubevirt.Get(runVM.Namespace, runVM.Name, metav1.GetOptions{})
	if err == nil {
		value, ok := existingVM.Labels[labelImported]
		if ok && value == "true" {
			found = true
		}
	}

	if !found {
		_, err := h.kubevirt.Create(runVM)
		if err != nil {
			return fmt.Errorf("error creating kubevirt VM %v in createVirtualMachine: %v", runVM, err)
		}
	}

	return nil
}

func (h *virtualMachineHandler) checkVirtualMachine(vm *migration.VirtualMachineImport) (bool, error) {
	vmObj, err := h.kubevirt.Get(vm.Namespace, vm.Status.ImportedVirtualMachineName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("error querying kubevirt vm in checkVirtualMachine: %v", err)
	}

	return vmObj.Status.Ready, nil
}

func (h *virtualMachineHandler) ReconcileVMI(_ string, _ string, obj runtime.Object) ([]relatedresource.Key, error) {
	if vmiObj, ok := obj.(*harvesterv1beta1.VirtualMachineImage); ok {
		owners := vmiObj.GetOwnerReferences()
		if vmiObj.DeletionTimestamp == nil {
			for _, v := range owners {
				if strings.EqualFold(v.Kind, "virtualmachineimport") {
					return []relatedresource.Key{
						{
							Namespace: vmiObj.Namespace,
							Name:      v.Name,
						},
					}, nil
				}
			}
		}
	}

	return nil, nil
}

func (h *virtualMachineHandler) cleanupAndResubmit(vm *migration.VirtualMachineImport) error {
	// need to wait for all VMI's to be complete or failed before we cleanup failed objects
	for i, d := range vm.Status.DiskImportStatus {
		if util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageFailed, corev1.ConditionTrue) {
			err := h.vmi.Delete(vm.Namespace, d.VirtualMachineImage, &metav1.DeleteOptions{})
			if err != nil {
				return fmt.Errorf("error deleting failed virtualmachineimage: %v", err)
			}
			conds := util.RemoveCondition(d.DiskConditions, migration.VirtualMachineImageFailed, corev1.ConditionTrue)
			d.DiskConditions = conds
			vm.Status.DiskImportStatus[i] = d
		}
	}

	return nil
}

func (h *virtualMachineHandler) findAndCreatePVC(vm *migration.VirtualMachineImport) error {
	for _, v := range vm.Status.DiskImportStatus {
		vmiObj, err := h.vmi.Get(vm.Namespace, v.VirtualMachineImage, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("error quering VirtualMachineImage '%s/%s' in findAndCreatePVC: %v", vm.Namespace, v.VirtualMachineImage, err)
		}

		// only needed for LonghornEngine v1, for cdi based images we will use the datavolume directly
		if vmiObj.Spec.Backend == harvesterv1beta1.VMIBackendBackingImage {
			// check if PVC has already been created
			createPVC := false
			pvcName := v.VirtualMachineImage
			_, err = h.pvc.Get(vm.Namespace, pvcName, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					createPVC = true
				} else {
					return fmt.Errorf("error looking up existing PVC '%s/%s' in findAndCreatePVC: %v", vm.Namespace, pvcName, err)
				}

			}

			if createPVC {
				pvcObj := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pvcName,
						Namespace: vm.Namespace,
						Annotations: map[string]string{
							harvesterutil.AnnotationImageID: fmt.Sprintf("%s/%s", vmiObj.Namespace, vmiObj.Name),
						},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteMany,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(fmt.Sprintf("%d", vmiObj.Status.Size)),
							},
						},
						StorageClassName: &vmiObj.Status.StorageClassName,
						VolumeMode:       &[]corev1.PersistentVolumeMode{corev1.PersistentVolumeBlock}[0],
					},
				}

				logrus.WithFields(logrus.Fields{
					"name":                  pvcObj.Name,
					"namespace":             pvcObj.Namespace,
					"annotations":           pvcObj.Annotations,
					"spec.storageClassName": *pvcObj.Spec.StorageClassName,
					"spec.volumeMode":       *pvcObj.Spec.VolumeMode,
				}).Info("Creating a new PVC")

				_, err = h.pvc.Create(pvcObj)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (h *virtualMachineHandler) tidyUpObjects(vm *migration.VirtualMachineImport) error {
	for _, v := range vm.Status.DiskImportStatus {
		vmiObj, err := h.vmi.Get(vm.Namespace, v.VirtualMachineImage, metav1.GetOptions{})
		if err != nil {
			return err
		}

		var newRef []metav1.OwnerReference
		for _, o := range vmiObj.GetOwnerReferences() {
			if o.Kind == vm.Kind && o.APIVersion == vm.APIVersion && o.UID == vm.UID && o.Name == vm.Name {
				continue
			}
			newRef = append(newRef, o)
		}

		vmiObj.ObjectMeta.OwnerReferences = newRef

		_, err = h.vmi.Update(vmiObj)
		if err != nil {
			return fmt.Errorf("error removing ownerReference for VirtualMachineImage %s/%s: %w", vmiObj.Namespace, vmiObj.Name, err)
		}
	}

	return nil
}

func (h *virtualMachineHandler) triggerCleanup(vmi *migration.VirtualMachineImport) error {
	vmo, err := h.generateVMO(vmi)
	if err != nil {
		return fmt.Errorf("failed to generate VMO in triggerCleanup: %w", err)
	}

	return vmo.Cleanup(vmi)
}

func (h *virtualMachineHandler) checkAndCreateVirtualMachineImage(vm *migration.VirtualMachineImport, d migration.DiskInfo) (*harvesterv1beta1.VirtualMachineImage, error) {
	displayName := fmt.Sprintf("vm-import-%s-%s", vm.Name, d.Name)

	// Make sure the label meets the standards for a Kubernetes label value.
	labelDisplayName := capiformat.MustFormatValue(displayName)

	// Check if the VirtualMachineImage object already exists.
	imageList, err := h.vmi.Cache().List(vm.Namespace, labels.SelectorFromSet(map[string]string{
		labelImageDisplayName: labelDisplayName,
	}))
	if err != nil {
		return nil, err
	}

	numImages := len(imageList)
	if numImages > 1 {
		return nil, fmt.Errorf("found %d Harvester VirtualMachineImages with label '%s=%s', only expected to find one", numImages, labelImageDisplayName, labelDisplayName)
	}
	if numImages == 1 {
		return imageList[0], nil
	}

	// No VirtualMachineImage object found. Create a new one and return the object.
	vmi := &harvesterv1beta1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "image-",
			Namespace:    vm.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: vm.APIVersion,
					Kind:       vm.Kind,
					UID:        vm.UID,
					Name:       vm.Name,
				},
			},
			Labels: map[string]string{
				labelImported: "true",
				// Set the `harvesterhci.io/imageDisplayName` label to be
				// able to search for the `VirtualMachineImage` object during
				// the reconciliation phase. See code above at the beginning
				// of this function.
				labelImageDisplayName: labelDisplayName,
			},
		},
		Spec: harvesterv1beta1.VirtualMachineImageSpec{
			DisplayName: displayName,
			URL:         fmt.Sprintf("http://%s:%d/%s", server.Address(), server.DefaultPort(), d.Name),
			SourceType:  "download",
		},
	}

	if vm.Spec.StorageClass != "" {
		vmiBackend, err := util.GetBackendFromStorageClassName(vm.Spec.StorageClass, h.sc)
		if err != nil {
			return nil, fmt.Errorf("failed to get VMI backend from storage class '%s': %v", vm.Spec.StorageClass, err)
		}

		if vmi.Annotations == nil {
			vmi.Annotations = make(map[string]string)
		}

		vmi.Annotations[harvesterutil.AnnotationStorageClassName] = vm.Spec.StorageClass
		vmi.Spec.Backend = vmiBackend
		vmi.Spec.TargetStorageClassName = vm.Spec.StorageClass
	}

	logrus.WithFields(logrus.Fields{
		"generateName":     vmi.GenerateName,
		"namespace":        vmi.Namespace,
		"annotations":      vmi.Annotations,
		"labels":           vmi.Labels,
		"ownerReferences":  vmi.OwnerReferences,
		"spec.displayName": vmi.Spec.DisplayName,
	}).Info("Creating a new Harvester VirtualMachineImage")

	vmiObj, err := h.vmi.Create(vmi)
	if err != nil {
		return nil, fmt.Errorf("failed to create Harvester VirtualMachineImage (namespace=%s spec.displayName=%s): %v", vmi.Namespace, vmi.Spec.DisplayName, err)
	}

	return vmiObj, nil
}

func (h *virtualMachineHandler) sanitizeVirtualMachineImport(vm *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	vmo, err := h.generateVMO(vm)
	if err != nil {
		return nil, fmt.Errorf("error generating VMO in sanitizeVirtualMachineImport: %w", err)
	}

	err = vmo.SanitizeVirtualMachineImport(vm)
	if err != nil {
		vm.Status.Status = migration.VirtualMachineImportInvalid
		logrus.WithFields(logrus.Fields{
			"kind":                              vm.Kind,
			"name":                              vm.Name,
			"namespace":                         vm.Namespace,
			"spec.virtualMachineName":           vm.Spec.VirtualMachineName,
			"status.importedVirtualMachineName": vm.Status.ImportedVirtualMachineName,
		}).Errorf("Failed to sanitize the import spec: %v", err)
	} else {
		// Make sure the ImportedVirtualMachineName is RFC 1123 compliant.
		if errs := validation.IsDNS1123Label(vm.Status.ImportedVirtualMachineName); len(errs) != 0 {
			vm.Status.Status = migration.VirtualMachineImportInvalid
			logrus.WithFields(logrus.Fields{
				"kind":                              vm.Kind,
				"name":                              vm.Name,
				"namespace":                         vm.Namespace,
				"spec.virtualMachineName":           vm.Spec.VirtualMachineName,
				"status.importedVirtualMachineName": vm.Status.ImportedVirtualMachineName,
			}).Error("The definitive name of the imported VM is not RFC 1123 compliant")
		} else {
			vm.Status.Status = migration.SourceReady
			logrus.WithFields(logrus.Fields{
				"kind":                              vm.Kind,
				"name":                              vm.Name,
				"namespace":                         vm.Namespace,
				"spec.virtualMachineName":           vm.Spec.VirtualMachineName,
				"status.importedVirtualMachineName": vm.Status.ImportedVirtualMachineName,
			}).Info("The sanitization of the import spec was successful")
		}
	}

	return h.importVM.UpdateStatus(vm)
}
