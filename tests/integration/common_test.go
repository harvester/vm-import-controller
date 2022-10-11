package integration

import (
	"fmt"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("perform valid dns names", func() {
	var creds *corev1.Secret
	var vcsim *migration.VmwareSource
	var vm *migration.VirtualMachineImport

	BeforeEach(func() {
		creds = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vcsim-creds",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "user",
				"password": "pass",
			},
		}

		vcsim = &migration.VmwareSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-vm-validation",
				Namespace: "default",
			},
			Spec: migration.VmwareSourceSpec{
				EndpointAddress: "",
				Datacenter:      "DC0",
				Credentials: corev1.SecretReference{
					Name:      creds.Name,
					Namespace: creds.Namespace,
				},
			},
		}

		vm = &migration.VirtualMachineImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm-validation",
				Namespace: "default",
			},
			Spec: migration.VirtualMachineImportSpec{
				SourceCluster: corev1.ObjectReference{
					Name:       vcsim.Name,
					Namespace:  vcsim.Namespace,
					Kind:       vcsim.Kind,
					APIVersion: vcsim.APIVersion,
				},
				VirtualMachineName: "someRandomName",
				Folder:             "",
			},
		}

		err := k8sClient.Create(ctx, creds)
		Expect(err).ToNot(HaveOccurred())
		vcsim.Spec.EndpointAddress = fmt.Sprintf("https://localhost:%s/sdk", vcsimPort)
		err = k8sClient.Create(ctx, vcsim)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Create(ctx, vm)
		Expect(err).ToNot(HaveOccurred())
	})

	It("check virtualmachine import has failed", func() {
		By("checking state of VM Import", func() {
			Eventually(func() error {
				vmObj := &migration.VirtualMachineImport{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: vm.Name,
					Namespace: vm.Namespace}, vmObj)
				if err != nil {
					return err
				}

				if vmObj.Status.Status == migration.VirtualMachineInvalid {
					return nil
				}

				return fmt.Errorf("waiiting for vm obj to be marked invalid")
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, creds)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Delete(ctx, vcsim)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Delete(ctx, vm)
		Expect(err).ToNot(HaveOccurred())
	})

})
