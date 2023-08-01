package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

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

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvester "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	kubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/ref"
	"github.com/harvester/vm-import-controller/pkg/apis/common"
	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	migrationController "github.com/harvester/vm-import-controller/pkg/generated/controllers/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/harvester/vm-import-controller/pkg/source/openstack"
	"github.com/harvester/vm-import-controller/pkg/source/vmware"
	"github.com/harvester/vm-import-controller/pkg/util"
)

const (
	vmiAnnotation    = "migaration.harvesterhci.io/virtualmachineimport"
	imageDisplayName = "harvesterhci.io/imageDisplayName"
)

type VirtualMachineOperations interface {
	// ExportVirtualMachine is responsible for generating the raw images for each disk associated with the VirtualMachineImport
	// Any image format conversion will be performed by the VM Operation
	ExportVirtualMachine(vm *migration.VirtualMachineImport) error

	// PowerOffVirtualMachine is responsible for the powering off the virtualmachine
	PowerOffVirtualMachine(vm *migration.VirtualMachineImport) error

	// IsPoweredOff will check the status of VM Power and return true if machine is powered off
	IsPoweredOff(vm *migration.VirtualMachineImport) (bool, error)

	GenerateVirtualMachine(vm *migration.VirtualMachineImport) (*kubevirt.VirtualMachine, error)
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
}

func RegisterVMImportController(ctx context.Context, vmware migrationController.VmwareSourceController, openstack migrationController.OpenstackSourceController,
	secret coreControllers.SecretController, importVM migrationController.VirtualMachineImportController, vmi harvester.VirtualMachineImageController, kubevirt kubevirtv1.VirtualMachineController, pvc coreControllers.PersistentVolumeClaimController) {

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
	importVM.OnChange(ctx, "virtualmachine-import-job-preflight-checks", vmHandler.preFlightChecksHandlers)
	importVM.OnChange(ctx, "virtualmachine-import-job-trigger-exports", vmHandler.triggerExportHandler)
	importVM.OnChange(ctx, "virtualmachine-import-job-create-virtualmachineimages", vmHandler.createVirtualMachineImageHandler)
	importVM.OnChange(ctx, "virtualmachine-import-job-reconcile-virtualmachineimages", vmHandler.reconcileVirtualMachineImageHandler)
	importVM.OnChange(ctx, "virtualmachine-import-job-clean-and-resubmit-images", vmHandler.cleanupAndResubmitHandler)
	importVM.OnChange(ctx, "virtualmachine-import-job-create-virtualmachine", vmHandler.createVirtualMachineHandler)
	importVM.OnChange(ctx, "virtualmachine-import-job-check-virtualmachine", vmHandler.checkVirtualMachineHandler)
}

func (h *virtualMachineHandler) preFlightChecksHandlers(key string, vmObj *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {

	if vmObj == nil || vmObj.DeletionTimestamp != nil {
		return nil, nil
	}

	vm := vmObj.DeepCopy()

	if vm.Status.Status != "" {
		return vm, nil
	}

	err := h.preFlightChecks(vm)
	if err != nil {
		if err.Error() == migration.NotValidDNS1123Label {
			logrus.Errorf("vm migration target %s in VM %s in namespace %s is not RFC 1123 compliant", vm.Spec.VirtualMachineName, vm.Name, vm.Namespace)
			vm.Status.Status = migration.VirtualMachineInvalid
			h.importVM.UpdateStatus(vm)
		} else {
			return vm, err
		}
	}
	vm.Status.Status = migration.SourceReady
	return h.importVM.UpdateStatus(vm)
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

	return nil
}

func (h *virtualMachineHandler) triggerExport(vm *migration.VirtualMachineImport) error {
	vmo, err := h.generateVMO(vm)
	if err != nil {
		return fmt.Errorf("error generating VMO in trigger export: %v", err)
	}

	// power off machine
	if !util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachinePoweringOff, v1.ConditionTrue) {
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
		!util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineExported, v1.ConditionTrue) {
		err := vmo.ExportVirtualMachine(vm)
		if err != nil {
			return fmt.Errorf("error exporting virtual machine: %v", err)
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
	var schemaRef = annotationSchemaOwners[schemaID]
	schemaRef = ref.AnnotationSchemaReference{SchemaID: schemaID, References: ref.NewAnnotationSchemaOwnerReferences()}
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
	return h.vmi.Create(vmi)
}

func (h *virtualMachineHandler) preFlightWrapper(vm *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	err := h.preFlightChecks(vm)
	if err != nil {
		if err.Error() == migration.NotValidDNS1123Label {
			logrus.Errorf("vm migration target %s in VM %s in namespace %s is not RFC 1123 compliant", vm.Spec.VirtualMachineName, vm.Name, vm.Namespace)
			vm.Status.Status = migration.VirtualMachineInvalid
			h.importVM.UpdateStatus(vm)
		} else {
			return vm, err
		}
	}
	vm.Status.Status = migration.SourceReady
	return h.importVM.UpdateStatus(vm)
}

func (h *virtualMachineHandler) triggerExportHandler(key string, vmObj *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	if vmObj == nil || vmObj.DeletionTimestamp != nil {
		return nil, nil
	}

	vm := vmObj.DeepCopy()

	if vm.Status.Status != migration.SourceReady {
		return vm, nil
	}
	err := h.triggerExport(vm)
	if err != nil {
		return vm, err
	}
	if util.ConditionExists(vm.Status.ImportConditions, migration.VirtualMachineExported, v1.ConditionTrue) {
		vm.Status.Status = migration.DisksExported
	}
	return h.importVM.UpdateStatus(vm)
}

func (h *virtualMachineHandler) createVirtualMachineImageHandler(key string, vmObj *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	if vmObj == nil || vmObj.DeletionTimestamp != nil {
		return nil, nil
	}

	vm := vmObj.DeepCopy()

	if vm.Status.Status != migration.DisksExported {
		return vm, nil
	}

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

func (h *virtualMachineHandler) reconcileVirtualMachineImageHandler(key string, vmObj *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	// check and update disk image status based on VirtualMachineImage watches
	if vmObj == nil || vmObj.DeletionTimestamp != nil {
		return nil, nil
	}

	vm := vmObj.DeepCopy()

	if vm.Status.Status != migration.DiskImagesSubmitted {
		return vm, nil
	}

	err := h.reconcileVMIStatus(vm)
	if err != nil {
		return vm, err
	}
	ok := true
	failed := false
	var failedCount, passedCount int
	for _, d := range vm.Status.DiskImportStatus {
		ok = util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageReady, v1.ConditionTrue) && ok
		if ok {
			passedCount++
		}
		failed = util.ConditionExists(d.DiskConditions, migration.VirtualMachineImageFailed, v1.ConditionTrue) || failed
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
		vm.Status.Status = migration.DiskImagesReady
	}

	if failed {
		vm.Status.Status = migration.DiskImagesFailed
	}
	return h.importVM.UpdateStatus(vm)
}

func (h *virtualMachineHandler) cleanupAndResubmitHandler(key string, vmObj *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	// re-export VM and trigger re-import again
	if vmObj == nil || vmObj.DeletionTimestamp != nil {
		return nil, nil
	}

	vm := vmObj.DeepCopy()

	if vm.Status.Status != migration.DiskImagesFailed {
		return vm, nil
	}

	err := h.cleanupAndResubmit(vm)
	if err != nil {
		return vm, err
	}
	vm.Status.Status = migration.SourceReady
	return h.importVM.UpdateStatus(vm)
}

func (h *virtualMachineHandler) createVirtualMachineHandler(key string, vmObj *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	if vmObj == nil || vmObj.DeletionTimestamp != nil {
		return nil, nil
	}

	vm := vmObj.DeepCopy()

	if vm.Status.Status != migration.DiskImagesReady {
		return vm, nil
	}
	// create VM to use the VirtualMachineObject
	err := h.createVirtualMachine(vm)
	if err != nil {
		return vm, err
	}
	vm.Status.Status = migration.VirtualMachineCreated
	return h.importVM.UpdateStatus(vm)
}

func (h *virtualMachineHandler) checkVirtualMachineHandler(key string, vmObj *migration.VirtualMachineImport) (*migration.VirtualMachineImport, error) {
	if vmObj == nil || vmObj.DeletionTimestamp != nil {
		return nil, nil
	}

	vm := vmObj.DeepCopy()

	if vm.Status.Status != migration.VirtualMachineCreated {
		return vm, nil
	}

	// wait for VM to be running using a watch on VM's
	ok, err := h.checkVirtualMachine(vm)
	if err != nil {
		return vm, err
	}

	if !ok {
		h.importVM.EnqueueAfter(vm.Namespace, vm.Name, 5*time.Minute)
		return vm, nil
	}
	vm.Status.Status = migration.VirtualMachineRunning
	return h.importVM.UpdateStatus(vm)
}
