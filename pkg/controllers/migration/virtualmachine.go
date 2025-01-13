package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/harvester/vm-import-controller/pkg/apis/common"
	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	migrationController "github.com/harvester/vm-import-controller/pkg/generated/controllers/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/harvester/vm-import-controller/pkg/source/openstack"
	"github.com/harvester/vm-import-controller/pkg/source/vmware"
	"github.com/harvester/vm-import-controller/pkg/util"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvester "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	kubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	coreControllers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/relatedresource"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	kubevirt "kubevirt.io/api/core/v1"

	storageControllers "github.com/rancher/wrangler/pkg/generated/controllers/storage/v1"
)

const (
	vmiAnnotation    = "migration.harvesterhci.io/virtualmachineimport"
	imageDisplayName = "harvesterhci.io/imageDisplayName"
)

type VirtualMachineOperations interface {
	// ExportVirtualMachine is responsible for generating the raw images for each disk associated with the VirtualMachineImport
	// Any image format conversion will be performed by the VM Operation
	ExportVirtualMachine(vm *migration.VirtualMachineImport) error

	// PowerOffVirtualMachine is responsible for the powering off the virtual machine
	PowerOffVirtualMachine(vm *migration.VirtualMachineImport) error

	// IsPoweredOff will check the status of VM Power and return true if machine is powered off
	IsPoweredOff(vm *migration.VirtualMachineImport) (bool, error)

	GenerateVirtualMachine(vm *migration.VirtualMachineImport) (*kubevirt.VirtualMachine, error)

	// PreFlightChecks checks the cluster specific configurations.
	PreFlightChecks(vm *migration.VirtualMachineImport) error
}

type virtualMachineHandler struct {
	ctx       context.Context
	vmware    migrationController.VmwareSourceController
	openstack migrationController.OpenstackSourceController
	secret    coreControllers.SecretController
	importVM  migrationController.VirtualMachineImportController
	vmi       harvester.VirtualMachineImageController
	kubevirt  kubevirtv1.VirtualMachineController
	pvc       coreControllers.PersistentVolumeClaimController
	sc        storageControllers.StorageClassCache
	nadCache  ctlcniv1.NetworkAttachmentDefinitionCache
}

func RegisterVMImportController(ctx context.Context, vmware migrationController.VmwareSourceController, openstack migrationController.OpenstackSourceController,
	secret coreControllers.SecretController, importVM migrationController.VirtualMachineImportController, vmi harvester.VirtualMachineImageController, kubevirt kubevirtv1.VirtualMachineController, pvc coreControllers.PersistentVolumeClaimController, scCache storageControllers.StorageClassCache, nadCache ctlcniv1.NetworkAttachmentDefinitionCache) {

	vmHandler := &virtualMachineHandler{
		ctx:       ctx,
		vmware:    vmware,
		openstack: openstack,
		secret:    secret,
		importVM:  importVM,
		vmi:       vmi,
		kubevirt:  kubevirt,
		pvc:       pvc,
		sc:        scCache,
		nadCache:  nadCache,
	}

	relatedresource.Watch(ctx, "virtualmachineimage-change", vmHandler.ReconcileVMI, importVM, vmi)
	importVM.OnChange(ctx, "virtualmachine-import-job-change", vmHandler.OnVirtualMachineChange)
}

func (h *virtualMachineHandler) OnVirtualMachineChange(_ string, vmObj *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {

	if vmObj == nil || vmObj.DeletionTimestamp != nil {
		return nil, nil
	}

	vm := vmObj.DeepCopy()
	switch vm.Status.Status {
	case "":
		// run preflight checks and make vm ready for import
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Info("Running preflight checks ...")
		return h.reconcilePreFlightChecks(vm)
	case migration.SourceReady:
		// vm migration is valid and ready. trigger migration specific import
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Info("Importing client disk images ...")
		return h.runVirtualMachineExport(vm)
	case migration.DisksExported:
		// prepare and add routes for disks to be used for VirtualMachineImage CRD
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Info("Creating VM images ...")
		return h.reconcileDiskImageStatus(vm)
	case migration.DiskImagesSubmitted:
		// check and update disk image status based on VirtualMachineImage watches
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Info("Evaluating VM images ...")
		err := h.reconcileVMIStatus(vm)
		if err != nil {
			return vm, err
		}
		newStatus := evaluateDiskImportStatus(vm.Status.DiskImportStatus)
		if newStatus == nil {
			return vm, nil
		}
		vm.Status.Status = *newStatus
		return h.importVM.UpdateStatus(vm)
	case migration.DiskImagesFailed:
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Error("Failed to import client disk images. Try again ...")
		return h.triggerResubmit(vm)
	case migration.DiskImagesReady:
		// create VM to use the VirtualMachineObject
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Info("Creating VM instances ...")
		err := h.createVirtualMachine(vm)
		if err != nil {
			return vm, err
		}
		vm.Status.Status = migration.VirtualMachineCreated
		return h.importVM.UpdateStatus(vm)
	case migration.VirtualMachineCreated:
		// wait for VM to be running using a watch on VM's
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Info("Checking VM instances ...")
		return h.reconcileVirtualMachineStatus(vm)
	case migration.VirtualMachineRunning:
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Info("The VM was imported successfully")
		return vm, h.tidyUpObjects(vm)
	case migration.VirtualMachineInvalid:
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Error("The VM import spec is invalid")
		return vm, nil
	case migration.VirtualMachineMigrationFailed:
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Error("The VM import has failed")
		return vm, nil
	}

	return vm, nil
}

// preFlightChecks is used to validate that the associate sources and VM migration references are valid
func (h *virtualMachineHandler) preFlightChecks(vm *migration.VirtualMachineImport) error {

	if errs := validation.IsDNS1123Label(vm.Spec.VirtualMachineName); len(errs) != 0 {
		return fmt.Errorf(migration.NotValidDNS1123Label)
	}

	if vm.Spec.SourceCluster.APIVersion != "migration.harvesterhci.io/v1beta1" {
		return fmt.Errorf("expected migration cluster apiversion to be migration.harvesterhci.io/v1beta1 but got %s", vm.Spec.SourceCluster.APIVersion)
	}

	var ss migration.SourceInterface
	var err error

	switch strings.ToLower(vm.Spec.SourceCluster.Kind) {
	case "vmwaresource", "openstacksource":
		ss, err = h.generateSource(vm)
		if err != nil {
			return fmt.Errorf("error generating migration in preflight checks :%v", err)
		}
	default:
		return fmt.Errorf("unsupported migration kind. Currently supported values are vmware/openstack but got %s", strings.ToLower(vm.Spec.SourceCluster.Kind))
	}

	if ss.ClusterStatus() != migration.ClusterReady {
		return fmt.Errorf("migration not yet ready. current status is %s", ss.ClusterStatus())
	}

	// verify specified storage class exists. Empty storage class means default storage class
	if vm.Spec.StorageClass != "" {
		_, err := h.sc.Get(vm.Spec.StorageClass)
		if err != nil {
			logrus.Errorf("error looking up storageclass %s: %v", vm.Spec.StorageClass, err)
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
		return fmt.Errorf("source network %s appears multiple times in vm spec", network.SourceNetwork)
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
					"spec.sourcecluster.kind": vm.Spec.SourceCluster.Kind,
				}).Errorf("Failed to get destination network '%s/%s': %v",
					parts[0], parts[1], err)
				return err
			}
		default:
			logrus.WithFields(logrus.Fields{
				"name":                    vm.Name,
				"namespace":               vm.Namespace,
				"spec.sourcecluster.kind": vm.Spec.SourceCluster.Kind,
			}).Errorf("Invalid destination network '%s'", nm.DestinationNetwork)
			return fmt.Errorf("invalid destination network '%s'", nm.DestinationNetwork)
		}
	}

	// Validate the source network as part of the source cluster preflight
	// checks.
	vmo, err := h.generateVMO(vm)
	if err != nil {
		return fmt.Errorf("error generating VMO in preFlightChecks: %v", err)
	}
	err = vmo.PreFlightChecks(vm)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.sourcecluster.kind": vm.Spec.SourceCluster.Kind,
		}).Errorf("Failed to perform source cluster specific preflight checks: %v", err)
		return err
	}

	return nil
}

func (h *virtualMachineHandler) triggerExport(vm *migration.VirtualMachineImport) error {
	vmo, err := h.generateVMO(vm)
	if err != nil {
		return fmt.Errorf("error generating VMO in trigger export: %v", err)
	}

	// power off machine
	if !util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachinePoweringOff, v1.ConditionTrue) {
		logrus.WithFields(logrus.Fields{
			"name":                    vm.Name,
			"namespace":               vm.Namespace,
			"spec.virtualMachineName": vm.Spec.VirtualMachineName,
		}).Info("Powering off client VM ...")
		err = vmo.PowerOffVirtualMachine(vm)
		if err != nil {
			return fmt.Errorf("error in poweroff call: %v", err)
		}
		conds := []common.Condition{
			{
				Type:               migration.VirtualMachinePoweringOff,
				Status:             v1.ConditionTrue,
				LastUpdateTime:     metav1.Now().Format(time.RFC3339),
				LastTransitionTime: metav1.Now().Format(time.RFC3339),
			},
		}
		vm.Status.ImportConditions = util.MergeConditions(vm.Status.ImportConditions, conds)
		return nil
	}

	if !util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachinePoweredOff, v1.ConditionTrue) &&
		util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachinePoweringOff, v1.ConditionTrue) {
		// check if VM is powered off
		ok, err := vmo.IsPoweredOff(vm)
		if err != nil {
			return fmt.Errorf("error during check for vm power: %v", err)
		}
		if ok {
			conds := []common.Condition{
				{
					Type:               migration.VirtualMachinePoweredOff,
					Status:             v1.ConditionTrue,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
				},
			}
			vm.Status.ImportConditions = util.MergeConditions(vm.Status.ImportConditions, conds)
			return nil
		}

		// default behaviour
		return fmt.Errorf("waiting for vm %s to be powered off", fmt.Sprintf("%s/%s", vm.Namespace, vm.Name))
	}

	if util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachinePoweredOff, v1.ConditionTrue) &&
		util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachinePoweringOff, v1.ConditionTrue) &&
		!util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineExported, v1.ConditionTrue) &&
		!util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineExportFailed, v1.ConditionTrue) {
		err := vmo.ExportVirtualMachine(vm)
		if err != nil {
			// avoid retrying if vm export fails
			conds := []common.Condition{
				{
					Type:               migration.VirtualMachineExportFailed,
					Status:             v1.ConditionTrue,
					LastUpdateTime:     metav1.Now().Format(time.RFC3339),
					LastTransitionTime: metav1.Now().Format(time.RFC3339),
					Message:            fmt.Sprintf("error exporting VM: %v", err),
				},
			}
			vm.Status.ImportConditions = util.MergeConditions(vm.Status.ImportConditions, conds)
			logrus.Errorf("error exporting virtualmachine %s for virtualmachineimport %s-%s: %v", vm.Spec.VirtualMachineName, vm.Namespace, vm.Name, err)
			return nil
		}
		conds := []common.Condition{
			{
				Type:               migration.VirtualMachineExported,
				Status:             v1.ConditionTrue,
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
		return nil, fmt.Errorf("error generating migration interface: %v", err)
	}

	secretRef := source.SecretReference()
	secret, err := h.secret.Get(secretRef.Namespace, secretRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error fetching secret :%v", err)
	}

	// generate VirtualMachineOperations Interface.
	// this will be used for migration specific operations

	if source.GetKind() == strings.ToLower("vmwaresource") {
		endpoint, dc := source.GetConnectionInfo()
		return vmware.NewClient(h.ctx, endpoint, dc, secret)
	}

	if source.GetKind() == strings.ToLower("openstacksource") {
		endpoint, region := source.GetConnectionInfo()
		return openstack.NewClient(h.ctx, endpoint, region, secret)
	}

	return nil, fmt.Errorf("unsupport source kind")
}

func (h *virtualMachineHandler) generateSource(vm *migration.VirtualMachineImport) (migration.SourceInterface, error) {
	var s migration.SourceInterface
	var err error
	if strings.ToLower(vm.Spec.SourceCluster.Kind) == "vmwaresource" {
		s, err = h.vmware.Get(vm.Spec.SourceCluster.Namespace, vm.Spec.SourceCluster.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}
	if strings.ToLower(vm.Spec.SourceCluster.Kind) == "openstacksource" {
		s, err = h.openstack.Get(vm.Spec.SourceCluster.Namespace, vm.Spec.SourceCluster.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (h *virtualMachineHandler) createVirtualMachineImages(vm *migration.VirtualMachineImport) error {
	// check and create VirtualMachineImage objects
	status := vm.Status.DeepCopy()
	for i, d := range status.DiskImportStatus {
		if !util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageSubmitted, v1.ConditionTrue) {
			vmiObj, err := h.checkAndCreateVirtualMachineImage(vm, d)
			if err != nil {
				return fmt.Errorf("error creating vmi: %v", err)
			}
			d.VirtualMachineImage = vmiObj.Name
			vm.Status.DiskImportStatus[i] = d
			cond := []common.Condition{
				{
					Type:               migration.VirtualMachineImageSubmitted,
					Status:             v1.ConditionTrue,
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
		if !util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageReady, v1.ConditionTrue) {
			vmi, err := h.vmi.Get(vm.Namespace, d.VirtualMachineImage, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("error quering vmi in reconcileVMIStatus: %v", err)
			}
			for _, v := range vmi.Status.Conditions {
				if v.Type == harvesterv1beta1.ImageImported && v.Status == v1.ConditionTrue {
					cond := []common.Condition{
						{
							Type:               migration.VirtualMachineImageReady,
							Status:             v1.ConditionTrue,
							LastUpdateTime:     metav1.Now().Format(time.RFC3339),
							LastTransitionTime: metav1.Now().Format(time.RFC3339),
						},
					}
					d.DiskConditions = util.MergeConditions(d.DiskConditions, cond)
					vm.Status.DiskImportStatus[i] = d
				}

				// handle failed imports if any
				if v.Type == harvesterv1beta1.ImageImported && v.Status == v1.ConditionFalse && v.Reason == "ImportFailed" {
					cond := []common.Condition{
						{
							Type:               migration.VirtualMachineImageFailed,
							Status:             v1.ConditionTrue,
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
		return fmt.Errorf("error generating VMO in createVirtualMachine :%v", err)
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
		pvcName := strings.ToLower(strings.Split(v.Name, ".img")[0])
		vmVols = append(vmVols, kubevirt.Volume{
			Name: fmt.Sprintf("disk-%d", i),
			VolumeSource: kubevirt.VolumeSource{
				PersistentVolumeClaim: &kubevirt.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: v1.PersistentVolumeClaimVolumeSource{
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
	// apply virtualmachineimport annotation

	if runVM.GetAnnotations() == nil {
		runVM.Annotations = make(map[string]string)
	}
	runVM.Annotations[vmiAnnotation] = fmt.Sprintf("%s-%s", vm.Name, vm.Namespace)

	found := false
	existingVMO, err := h.kubevirt.Get(runVM.Namespace, runVM.Name, metav1.GetOptions{})
	if err == nil {
		if existingVMO.Annotations[vmiAnnotation] == fmt.Sprintf("%s-%s", vm.Name, vm.Namespace) {
			found = true
			vm.Status.NewVirtualMachine = existingVMO.Name
		}
	}

	if !found {
		runVMObj, err := h.kubevirt.Create(runVM)
		if err != nil {
			return fmt.Errorf("error creating kubevirt VM in createVirtualMachine :%v", err)
		}
		vm.Status.NewVirtualMachine = runVMObj.Name
	}

	return nil
}

func (h *virtualMachineHandler) checkVirtualMachine(vm *migration.VirtualMachineImport) (bool, error) {
	vmObj, err := h.kubevirt.Get(vm.Namespace, vm.Status.NewVirtualMachine, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("error querying kubevirt vm in checkVirtualMachine :%v", err)
	}

	return vmObj.Status.Ready, nil
}

func (h *virtualMachineHandler) ReconcileVMI(_ string, _ string, obj runtime.Object) ([]relatedresource.Key, error) {
	if vmiObj, ok := obj.(*harvesterv1beta1.VirtualMachineImage); ok {
		owners := vmiObj.GetOwnerReferences()
		if vmiObj.DeletionTimestamp == nil {
			for _, v := range owners {
				if strings.ToLower(v.Kind) == "virtualmachineimport" {
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
		if util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageFailed, v1.ConditionTrue) {
			err := h.vmi.Delete(vm.Namespace, d.VirtualMachineImage, &metav1.DeleteOptions{})
			if err != nil {
				return fmt.Errorf("error deleting failed virtualmachineimage: %v", err)
			}
			conds := util.RemoveCondition(d.DiskConditions, migration.VirtualMachineImageFailed, v1.ConditionTrue)
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
			return fmt.Errorf("error quering vmi in findAndCreatePVC :%v", err)
		}

		// check if PVC has already been created
		var createPVC bool
		pvcName := strings.ToLower(strings.Split(v.Name, ".img")[0])
		_, err = h.pvc.Get(vm.Namespace, pvcName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				createPVC = true
			} else {
				return fmt.Errorf("error looking up existing PVC in findAndCreatePVC :%v", err)
			}

		}

		if createPVC {
			annotations, err := generateAnnotations(vm, vmiObj)
			if err != nil {
				return err
			}

			pvcObj := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        pvcName,
					Namespace:   vm.Namespace,
					Annotations: annotations,
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{
						v1.ReadWriteMany,
					},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse(fmt.Sprintf("%d", vmiObj.Status.Size)),
						},
					},
					StorageClassName: &vmiObj.Status.StorageClassName,
					VolumeMode:       &[]v1.PersistentVolumeMode{v1.PersistentVolumeBlock}[0],
				},
			}

			_, err = h.pvc.Create(pvcObj)
			if err != nil {
				return err
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
			return fmt.Errorf("error removing ownerReference for vmi %s :%v", vmiObj.Name, err)
		}

		// remove processed img files
		os.Remove(filepath.Join(server.TempDir(), v.Name))
	}
	return nil
}

// generateAnnotations will generate the harvester reference annotations: github.com/harvester/harvester/pkg/ref
func generateAnnotations(vm *migration.VirtualMachineImport, vmi *harvesterv1beta1.VirtualMachineImage) (map[string]string, error) {
	annotationSchemaOwners := ref.AnnotationSchemaOwners{}
	_ = annotationSchemaOwners.Add(kubevirt.VirtualMachineGroupVersionKind.GroupKind(), vm)
	var schemaID = ref.GroupKindToSchemaID(kubevirt.VirtualMachineGroupVersionKind.GroupKind())
	var ownerRef = ref.Construct(vm.GetNamespace(), vm.Spec.VirtualMachineName)
	schemaRef := ref.AnnotationSchemaReference{SchemaID: schemaID, References: ref.NewAnnotationSchemaOwnerReferences()}
	schemaRef.References.Insert(ownerRef)
	annotationSchemaOwners[schemaID] = schemaRef
	var ownersBytes, err = json.Marshal(annotationSchemaOwners)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal annotation schema owners: %w", err)
	}
	annotations := map[string]string{
		ref.AnnotationSchemaOwnerKeyName: string(ownersBytes),
		"harvesterhci.io/imageId":        fmt.Sprintf("%s/%s", vmi.Namespace, vmi.Name),
	}
	return annotations, nil
}

func (h *virtualMachineHandler) checkAndCreateVirtualMachineImage(vm *migration.VirtualMachineImport, d migration.DiskInfo) (*harvesterv1beta1.VirtualMachineImage, error) {
	imageList, err := h.vmi.Cache().List(vm.Namespace, labels.SelectorFromSet(map[string]string{
		imageDisplayName: fmt.Sprintf("vm-import-%s-%s", vm.Name, d.Name),
	}))

	if err != nil {
		return nil, err
	}

	if len(imageList) > 1 {
		return nil, fmt.Errorf("unexpected error: found %d images with label %s=%s, only expected to find one", len(imageList), imageDisplayName, fmt.Sprintf("vm-import-%s-%s", vm.Name, d.Name))
	}

	if len(imageList) == 1 {
		return imageList[0], nil
	}

	// no image found create a new VMI and return object
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
				imageDisplayName: fmt.Sprintf("vm-import-%s-%s", vm.Name, d.Name),
			},
		},
		Spec: harvesterv1beta1.VirtualMachineImageSpec{
			DisplayName: fmt.Sprintf("vm-import-%s-%s", vm.Name, d.Name),
			URL:         fmt.Sprintf("http://%s:%d/%s", server.Address(), server.DefaultPort(), d.Name),
			SourceType:  "download",
		},
	}

	if vm.Spec.StorageClass != "" {
		// update storage class annotations
		vmi.Annotations = map[string]string{
			"harvesterhci.io/storageClassName": vm.Spec.StorageClass,
		}
	}

	return h.vmi.Create(vmi)
}
