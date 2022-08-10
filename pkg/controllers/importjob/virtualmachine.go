package importjob

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	kubevirt "kubevirt.io/api/core/v1"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvester "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	kubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/vm-import-controller/pkg/apis/common"
	importjob "github.com/harvester/vm-import-controller/pkg/apis/importjob.harvesterhci.io/v1beta1"
	source "github.com/harvester/vm-import-controller/pkg/apis/source.harvesterhci.io/v1beta1"
	importJobController "github.com/harvester/vm-import-controller/pkg/generated/controllers/importjob.harvesterhci.io/v1beta1"
	sourceController "github.com/harvester/vm-import-controller/pkg/generated/controllers/source.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/harvester/vm-import-controller/pkg/util"
	coreControllers "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/relatedresource"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type virtualMachineHandler struct {
	ctx       context.Context
	vmware    sourceController.VmwareController
	openstack sourceController.OpenstackController
	secret    coreControllers.SecretController
	importVM  importJobController.VirtualMachineController
	vmi       harvester.VirtualMachineImageController
	kubevirt  kubevirtv1.VirtualMachineController
	pvc       coreControllers.PersistentVolumeClaimController
}

func RegisterVMImportController(ctx context.Context, vmware sourceController.VmwareController, openstack sourceController.OpenstackController,
	secret coreControllers.SecretController, importVM importJobController.VirtualMachineController, vmi harvester.VirtualMachineImageController, kubevirt kubevirtv1.VirtualMachineController, pvc coreControllers.PersistentVolumeClaimController) {

	vmHandler := &virtualMachineHandler{
		ctx:       ctx,
		vmware:    vmware,
		openstack: openstack,
		secret:    secret,
		importVM:  importVM,
		vmi:       vmi,
		kubevirt:  kubevirt,
		pvc:       pvc,
	}

	relatedresource.Watch(ctx, "virtualmachineimage-change", vmHandler.ReconcileVMI, importVM, vmi)
	importVM.OnChange(ctx, "virtualmachine-import-job-change", vmHandler.OnVirtualMachineChange)
}

func (h *virtualMachineHandler) OnVirtualMachineChange(key string, vm *importjob.VirtualMachine) (*importjob.VirtualMachine, error) {

	if vm == nil || vm.DeletionTimestamp != nil {
		return vm, nil
	}

	switch vm.Status.Status {
	case "": // run preflight checks and make vm ready for import
		err := h.preFlightChecks(vm)
		if err != nil {
			return vm, err
		}
		vm.Status.Status = importjob.SourceReady
		return h.importVM.UpdateStatus(vm)
	case importjob.SourceReady: //vm source is valid and ready. trigger source specific import
		err := h.triggerExport(vm)
		if err != nil {
			return vm, err
		}
		if util.ConditionExists(vm.Status.ImportConditions, importjob.VirtualMachineExported, v1.ConditionTrue) {
			vm.Status.Status = importjob.DisksExported
		}
		return h.importVM.UpdateStatus(vm)
	case importjob.DisksExported: // prepare and add routes for disks to be used for VirtualMachineImage CRD
		orgStatus := vm.Status.DeepCopy()
		err := h.createVirtualMachineImages(vm)
		if err != nil {
			// check if any disks have been updated. We need to save this info to eventually reconcile the VMI creation
			var newVM *importjob.VirtualMachine
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
			ok = util.ConditionExists(d.DiskConditions, importjob.VirtualMachineImageSubmitted, v1.ConditionTrue) && ok
		}

		if ok {
			vm.Status.Status = importjob.DiskImagesSubmitted
		}
		return h.importVM.UpdateStatus(vm)
	case importjob.DiskImagesSubmitted:
		// check and update disk image status based on VirtualMachineImage watches
		err := h.reconcileVMIStatus(vm)
		if err != nil {
			return vm, err
		}
		ok := true
		failed := false
		var failedCount, passedCount int
		for _, d := range vm.Status.DiskImportStatus {
			ok = util.ConditionExists(d.DiskConditions, importjob.VirtualMachineImageReady, v1.ConditionTrue) && ok
			if ok {
				passedCount++
			}
			failed = util.ConditionExists(d.DiskConditions, importjob.VirtualMachineImageFailed, v1.ConditionTrue) || failed
			if failed {
				failedCount++
			}
		}

		if len(vm.Status.DiskImportStatus) != failedCount+passedCount {
			// if length's dont match, then we have disks with missing status. Lets ignore failures for now, and handle
			// disk failures once we have had watches triggered for all disks
			return vm, nil
		}

		if ok {
			vm.Status.Status = importjob.DiskImagesReady
		}

		if failed {
			vm.Status.Status = importjob.DiskImagesFailed
		}
		return h.importVM.UpdateStatus(vm)
	case importjob.DiskImagesFailed:
		// re-export VM and trigger re-import again
		err := h.cleanupAndResubmit(vm)
		if err != nil {
			return vm, err
		}
		vm.Status.Status = importjob.SourceReady
		return h.importVM.UpdateStatus(vm)
	case importjob.DiskImagesReady:
		// create VM to use the VirtualMachineObject
		err := h.createVirtualMachine(vm)
		if err != nil {
			return vm, err
		}
		vm.Status.Status = importjob.VirtualMachineCreated
		return h.importVM.UpdateStatus(vm)
	case importjob.VirtualMachineCreated:
		// wait for VM to be running using a watch on VM's
		ok, err := h.checkVirtualMachine(vm)
		if err != nil {
			return vm, err
		}
		if ok {
			vm.Status.Status = importjob.VirtualMachineRunning
			h.importVM.UpdateStatus(vm)
		}
		// by default we will poll again after 5 mins
		h.importVM.EnqueueAfter(vm.Namespace, vm.Name, 5*time.Minute)
	case importjob.VirtualMachineRunning:
		logrus.Infof("vm %s in namespace %v imported successfully", vm.Name, vm.Namespace)
		return vm, h.tidyUpObjects(vm)
	}

	return vm, nil
}

// preFlightChecks is used to validate that the associate sources and VM source references are valid
func (h *virtualMachineHandler) preFlightChecks(vm *importjob.VirtualMachine) error {
	if vm.Spec.SourceCluster.APIVersion != "source.harvesterhci.io/v1beta1" {
		return fmt.Errorf("expected source cluster apiversion to be source.harvesterhci.io/v1beta1 but got %s", vm.Spec.SourceCluster.APIVersion)
	}

	var ss source.SourceInterface
	var err error

	switch strings.ToLower(vm.Spec.SourceCluster.Kind) {
	case "vmware", "openstack":
		ss, err = h.generateSource(vm)
		if err != nil {
			return fmt.Errorf("error generating source in preflight checks :%v", err)
		}
	default:
		return fmt.Errorf("unsupported source kind. Currently supported values are vmware/openstack but got %s", strings.ToLower(vm.Spec.SourceCluster.Kind))
	}

	if ss.ClusterStatus() != source.ClusterReady {
		return fmt.Errorf("source not yet ready. current status is %s", ss.ClusterStatus())
	}

	return nil
}

func (h *virtualMachineHandler) triggerExport(vm *importjob.VirtualMachine) error {
	vmo, err := h.generateVMO(vm)
	if err != nil {
		return fmt.Errorf("error generating VMO in trigger export: %v", err)
	}

	// power off machine
	if !util.ConditionExists(vm.Status.ImportConditions, importjob.VirtualMachinePoweringOff, v1.ConditionTrue) {
		err = vmo.PowerOffVirtualMachine(vm)
		if err != nil {
			return fmt.Errorf("error in poweroff call: %v", err)
		}
		conds := []common.Condition{
			{
				Type:               importjob.VirtualMachinePoweringOff,
				Status:             v1.ConditionTrue,
				LastUpdateTime:     metav1.Now().Format(time.RFC3339),
				LastTransitionTime: metav1.Now().Format(time.RFC3339),
			},
		}
		vm.Status.ImportConditions = util.MergeConditions(vm.Status.ImportConditions, conds)
		return nil
	}

	if !util.ConditionExists(vm.Status.ImportConditions, importjob.VirtualMachinePoweredOff, v1.ConditionTrue) &&
		util.ConditionExists(vm.Status.ImportConditions, importjob.VirtualMachinePoweringOff, v1.ConditionTrue) {
		// check if VM is powered off
		ok, err := vmo.IsPoweredOff(vm)
		if err != nil {
			return fmt.Errorf("error during check for vm power: %v", err)
		}
		if ok {
			conds := []common.Condition{
				{
					Type:               importjob.VirtualMachinePoweredOff,
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

	if util.ConditionExists(vm.Status.ImportConditions, importjob.VirtualMachinePoweredOff, v1.ConditionTrue) &&
		util.ConditionExists(vm.Status.ImportConditions, importjob.VirtualMachinePoweringOff, v1.ConditionTrue) &&
		!util.ConditionExists(vm.Status.ImportConditions, importjob.VirtualMachineExported, v1.ConditionTrue) {
		err := vmo.ExportVirtualMachine(vm)
		if err != nil {
			return fmt.Errorf("error exporting virtual machine: %v", err)
		}
		conds := []common.Condition{
			{
				Type:               importjob.VirtualMachineExported,
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
func (h *virtualMachineHandler) generateVMO(vm *importjob.VirtualMachine) (source.VirtualMachineOperations, error) {

	source, err := h.generateSource(vm)
	if err != nil {
		return nil, fmt.Errorf("error generating source interface: %v", err)
	}

	secretRef := source.SecretReference()
	secret, err := h.secret.Get(secretRef.Namespace, secretRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error fetching secret :%v", err)
	}

	// generate VirtualMachineOperations Interface.
	// this will be used for source specific operations
	return source.GenerateClient(h.ctx, secret)
}

func (h *virtualMachineHandler) generateSource(vm *importjob.VirtualMachine) (source.SourceInterface, error) {
	var s source.SourceInterface
	var err error
	if strings.ToLower(vm.Spec.SourceCluster.Kind) == "vmware" {
		s, err = h.vmware.Get(vm.Spec.SourceCluster.Namespace, vm.Spec.SourceCluster.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}
	if strings.ToLower(vm.Spec.SourceCluster.Kind) == "openstack" {
		s, err = h.openstack.Get(vm.Spec.SourceCluster.Namespace, vm.Spec.SourceCluster.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (h *virtualMachineHandler) createVirtualMachineImages(vm *importjob.VirtualMachine) error {
	// check and create VirtualMachineImage objects
	status := vm.Status.DeepCopy()
	for i, d := range status.DiskImportStatus {
		if !util.ConditionExists(d.DiskConditions, importjob.VirtualMachineImageSubmitted, v1.ConditionTrue) {
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
				},
				Spec: harvesterv1beta1.VirtualMachineImageSpec{
					DisplayName: fmt.Sprintf("vm-import-%s-%s", vm.Name, d.Name),
					URL:         fmt.Sprintf("http://%s:%d/%s", server.Address(), server.DefaultPort(), d.Name),
					SourceType:  "download",
				},
			}
			vmiObj, err := h.vmi.Create(vmi)
			if err != nil {
				return fmt.Errorf("error creating vmi: %v", err)
			}
			d.VirtualMachineImage = vmiObj.Name
			vm.Status.DiskImportStatus[i] = d
			cond := []common.Condition{
				{
					Type:               importjob.VirtualMachineImageSubmitted,
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

func (h *virtualMachineHandler) reconcileVMIStatus(vm *importjob.VirtualMachine) error {
	for i, d := range vm.Status.DiskImportStatus {
		if !util.ConditionExists(d.DiskConditions, importjob.VirtualMachineImageReady, v1.ConditionTrue) {
			vmi, err := h.vmi.Get(vm.Namespace, d.VirtualMachineImage, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("error quering vmi in reconcileVMIStatus: %v", err)
			}
			for _, v := range vmi.Status.Conditions {
				if v.Type == harvesterv1beta1.ImageImported && v.Status == v1.ConditionTrue {
					cond := []common.Condition{
						{
							Type:               importjob.VirtualMachineImageReady,
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
							Type:               importjob.VirtualMachineImageFailed,
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

func (h *virtualMachineHandler) createVirtualMachine(vm *importjob.VirtualMachine) error {
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
	var vmVols []kubevirt.Volume
	var disks []kubevirt.Disk
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
			BootOrder: &[]uint{uint(diskOrder)}[0],
			DiskDevice: kubevirt.DiskDevice{
				Disk: &kubevirt.DiskTarget{
					Bus: "virtio",
				},
			},
		})
	}

	runVM.Spec.Template.Spec.Volumes = vmVols
	runVM.Spec.Template.Spec.Domain.Devices.Disks = disks
	runVMObj, err := h.kubevirt.Create(runVM)
	if err != nil {
		return fmt.Errorf("error creating kubevirt VM in createVirtualMachine :%v", err)
	}

	vm.Status.NewVirtualMachine = runVMObj.Name
	return nil
}

func (h *virtualMachineHandler) checkVirtualMachine(vm *importjob.VirtualMachine) (bool, error) {
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
				if strings.ToLower(v.Kind) == "virtualmachine" {
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

func (h *virtualMachineHandler) cleanupAndResubmit(vm *importjob.VirtualMachine) error {
	// need to wait for all VMI's to be complete or failed before we cleanup failed objects
	for i, d := range vm.Status.DiskImportStatus {
		if util.ConditionExists(d.DiskConditions, importjob.VirtualMachineImageFailed, v1.ConditionTrue) {
			err := h.vmi.Delete(vm.Namespace, d.VirtualMachineImage, &metav1.DeleteOptions{})
			if err != nil {
				return fmt.Errorf("error deleting failed virtualmachineimage: %v", err)
			}
			conds := util.RemoveCondition(d.DiskConditions, importjob.VirtualMachineImageFailed, v1.ConditionTrue)
			d.DiskConditions = conds
			vm.Status.DiskImportStatus[i] = d
		}
	}

	return nil
}

func (h *virtualMachineHandler) findAndCreatePVC(vm *importjob.VirtualMachine) error {
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
			pvcObj := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: vm.Namespace,
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

func (h *virtualMachineHandler) tidyUpObjects(vm *importjob.VirtualMachine) error {
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
