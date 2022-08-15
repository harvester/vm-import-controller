package integration

import (
	"fmt"
	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	importjob "github.com/harvester/vm-import-controller/pkg/apis/importjob.harvesterhci.io/v1beta1"
	source "github.com/harvester/vm-import-controller/pkg/apis/source.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/util"
	"github.com/harvester/vm-import-controller/tests/setup"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	kubevirt "kubevirt.io/api/core/v1"
	"strings"
)

var _ = Describe("test openstack export/import integration", func() {
	BeforeEach(func() {
		if !useExisting {
			return
		}
		err := setup.SetupOpenstack(ctx, k8sClient)
		Expect(err).ToNot(HaveOccurred())
	})

	It("reconcile openstack importjob object status", func() {
		if !useExisting {
			Skip("skipping openstack integration tests as not using an existing environment")
		}

		By("checking if openstack source is ready", func() {
			Eventually(func() error {
				o := &source.Openstack{}
				err := k8sClient.Get(ctx, setup.OpenstackSourceNamespacedName, o)
				if err != nil {
					return err
				}

				if o.Status.Status != source.ClusterReady {
					return fmt.Errorf("waiting for cluster source to be ready. current status is %s", o.Status.Status)
				}
				return nil
			}, "30s", "10s").ShouldNot(HaveOccurred())
		})

		By("vm importjob has the correct conditions", func() {
			Eventually(func() error {
				v := &importjob.VirtualMachine{}
				err := k8sClient.Get(ctx, setup.OpenstackVMNamespacedName, v)
				if err != nil {
					return err
				}
				if !util.ConditionExists(v.Status.ImportConditions, importjob.VirtualMachinePoweringOff, v1.ConditionTrue) {
					return fmt.Errorf("expected virtualmachinepoweringoff condition to be present")
				}

				if !util.ConditionExists(v.Status.ImportConditions, importjob.VirtualMachinePoweredOff, v1.ConditionTrue) {
					return fmt.Errorf("expected virtualmachinepoweredoff condition to be present")
				}

				if !util.ConditionExists(v.Status.ImportConditions, importjob.VirtualMachineExported, v1.ConditionTrue) {
					return fmt.Errorf("expected virtualmachineexported condition to be present")
				}

				return nil
			}, "300s", "10s").ShouldNot(HaveOccurred())
		})

		By("checking that PVC claim has been created", func() {
			Eventually(func() error {
				v := &importjob.VirtualMachine{}
				err := k8sClient.Get(ctx, setup.OpenstackVMNamespacedName, v)
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
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: setup.OpenstackVMNamespacedName.Namespace,
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
				v := &importjob.VirtualMachine{}
				err := k8sClient.Get(ctx, setup.OpenstackVMNamespacedName, v)
				if err != nil {
					return err
				}

				vm := &kubevirt.VirtualMachine{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Namespace: setup.OpenstackVMNamespacedName.Namespace,
					Name:      v.Spec.VirtualMachineName,
				}, vm)

				return err
			}, "300s", "10s").ShouldNot(HaveOccurred())
		})

		By("checking that the virtualmachineimage ownership has been removed", func() {
			Eventually(func() error {
				v := &importjob.VirtualMachine{}
				err := k8sClient.Get(ctx, setup.OpenstackVMNamespacedName, v)
				if err != nil {
					return err
				}

				for _, d := range v.Status.DiskImportStatus {
					vmi := &harvesterv1beta1.VirtualMachineImage{}
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: setup.OpenstackVMNamespacedName.Namespace,
						Name: d.VirtualMachineImage}, vmi)
					if err != nil {
						return err
					}

					if len(vmi.OwnerReferences) != 0 {
						return fmt.Errorf("waiting for ownerRef to be cleared")
					}
				}

				return nil
			}, "300s", "10s").ShouldNot(HaveOccurred())
		})
	})

	AfterEach(func() {
		if !useExisting {
			return
		}
		err := setup.CleanupOpenstack(ctx, k8sClient)
		Expect(err).ToNot(HaveOccurred())
	})
})
