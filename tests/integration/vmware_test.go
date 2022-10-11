package integration

import (
	"fmt"
	"strings"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/util"
	"github.com/harvester/vm-import-controller/tests/setup"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	kubevirt "kubevirt.io/api/core/v1"
)

var _ = Describe("test vmware export/import integration", func() {

	BeforeEach(func() {
		if !useExisting {
			return
		}
		err := setup.SetupVMware(ctx, k8sClient)
		Expect(err).ToNot(HaveOccurred())
	})

	It("reconcile object status", func() {
		if !useExisting {
			Skip("skipping vmware integration tests as not using an existing environment")
		}

		By("checking if vmware migration is ready", func() {
			Eventually(func() error {
				v := &migration.VmwareSource{}
				err := k8sClient.Get(ctx, setup.VmwareSourceNamespacedName, v)
				if err != nil {
					return err
				}
				if v.Status.Status != migration.ClusterReady {
					return fmt.Errorf("waiting for cluster migration to be ready. current condition is %s", v.Status.Status)
				}

				return nil
			}, "30s", "10s").ShouldNot(HaveOccurred())
		})

		By("vm importjob has the correct conditions", func() {
			Eventually(func() error {
				v := &migration.VirtualMachineImport{}
				err := k8sClient.Get(ctx, setup.VmwareVMNamespacedName, v)
				if err != nil {
					return err
				}
				if !util.ConditionExists(v.Status.ImportConditions, migration.VirtualMachinePoweringOff, v1.ConditionTrue) {
					return fmt.Errorf("expected virtualmachinepoweringoff condition to be present")
				}

				if !util.ConditionExists(v.Status.ImportConditions, migration.VirtualMachinePoweredOff, v1.ConditionTrue) {
					return fmt.Errorf("expected virtualmachinepoweredoff condition to be present")
				}

				if !util.ConditionExists(v.Status.ImportConditions, migration.VirtualMachineExported, v1.ConditionTrue) {
					return fmt.Errorf("expected virtualmachineexported condition to be present")
				}

				return nil
			}, "300s", "10s").ShouldNot(HaveOccurred())
		})

		By("checking status of virtualmachineimage objects", func() {
			Eventually(func() error {
				v := &migration.VirtualMachineImport{}
				err := k8sClient.Get(ctx, setup.VmwareVMNamespacedName, v)
				if err != nil {
					return err
				}

				if len(v.Status.DiskImportStatus) == 0 {
					return fmt.Errorf("waiting for DiskImportStatus to be populated")
				}
				for _, d := range v.Status.DiskImportStatus {
					if d.VirtualMachineImage == "" {
						return fmt.Errorf("waiting for VMI to be populated")
					}
					vmi := &harvesterv1beta1.VirtualMachineImage{}
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: setup.VmwareVMNamespacedName.Namespace,
						Name: d.VirtualMachineImage}, vmi)
					if err != nil {
						return err
					}

					if vmi.Status.Progress != 100 {
						return fmt.Errorf("vmi %s not yet ready", vmi.Name)
					}
				}
				return nil
			}, "300s", "10s").ShouldNot(HaveOccurred())
		})

		By("checking that PVC claim has been created", func() {
			Eventually(func() error {
				v := &migration.VirtualMachineImport{}
				err := k8sClient.Get(ctx, setup.VmwareVMNamespacedName, v)
				if err != nil {
					return err
				}
				if len(v.Status.DiskImportStatus) == 0 {
					return fmt.Errorf("diskimportstatus should have image details available")
				}
				for _, d := range v.Status.DiskImportStatus {
					if d.VirtualMachineImage == "" {
						return fmt.Errorf("waiting for VMI to be populated")
					}
					pvc := &v1.PersistentVolumeClaim{}
					pvcName := strings.ToLower(strings.Split(d.Name, ".img")[0])
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: setup.VmwareVMNamespacedName.Namespace,
						Name: pvcName}, pvc)
					if err != nil {
						return err
					}

					if pvc.Status.Phase != v1.ClaimBound {
						return fmt.Errorf("waiting for pvc claim to be in state bound")
					}
				}

				return nil
			}, "120s", "10s").ShouldNot(HaveOccurred())
		})

		By("checking that the virtualmachine has been created", func() {
			Eventually(func() error {
				v := &migration.VirtualMachineImport{}
				err := k8sClient.Get(ctx, setup.VmwareVMNamespacedName, v)
				if err != nil {
					return err
				}

				vm := &kubevirt.VirtualMachine{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Namespace: setup.VmwareVMNamespacedName.Namespace,
					Name:      v.Spec.VirtualMachineName,
				}, vm)

				return err
			}, "300s", "10s").ShouldNot(HaveOccurred())
		})

		// can take upto 5 mins for the VM to be marked as running
		By("checking that the virtualmachineimage ownership has been removed", func() {
			Eventually(func() error {
				v := &migration.VirtualMachineImport{}
				err := k8sClient.Get(ctx, setup.VmwareVMNamespacedName, v)
				if err != nil {
					return err
				}

				for _, d := range v.Status.DiskImportStatus {
					vmi := &harvesterv1beta1.VirtualMachineImage{}
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: setup.VmwareVMNamespacedName.Namespace,
						Name: d.VirtualMachineImage}, vmi)
					if err != nil {
						return err
					}

					if len(vmi.OwnerReferences) != 0 {
						return fmt.Errorf("waiting for ownerRef to be cleared")
					}
				}

				return nil
			}, "600s", "30s").ShouldNot(HaveOccurred())
		})

	})

	AfterEach(func() {
		if !useExisting {
			return
		}
		err := setup.CleanupVmware(ctx, k8sClient)
		Expect(err).ToNot(HaveOccurred())
	})
})
