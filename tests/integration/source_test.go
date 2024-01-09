package integration

import (
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/util"
)

var _ = Describe("verify vmware is ready", func() {
	var creds *corev1.Secret
	var vcsim *migration.VmwareSource

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
				Name:      "local",
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

		err := k8sClient.Create(ctx, creds)
		Expect(err).ToNot(HaveOccurred())
		vcsim.Spec.EndpointAddress = fmt.Sprintf("https://localhost:%s/sdk", vcsimPort)
		err = k8sClient.Create(ctx, vcsim)
		Expect(err).ToNot(HaveOccurred())
	})

	It("check vmware migration is ready", func() {
		// check status of migration object
		Eventually(func() error {
			vcsimObj := &migration.VmwareSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: vcsim.Name,
				Namespace: vcsim.Namespace}, vcsimObj)
			if err != nil {
				return err
			}

			if vcsimObj.Status.Status == migration.ClusterReady {
				return nil
			}

			return fmt.Errorf("migration currently in state: %v, expected to be %s", vcsimObj.Status.Status, migration.ClusterReady)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		// check conditions on migration object
		Eventually(func() error {
			vcsimObj := &migration.VmwareSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: vcsim.Name,
				Namespace: vcsim.Namespace}, vcsimObj)
			if err != nil {
				return err
			}
			if util.ConditionExists(vcsimObj.Status.Conditions, migration.ClusterReadyCondition, corev1.ConditionTrue) &&
				util.ConditionExists(vcsimObj.Status.Conditions, migration.ClusterErrorCondition, corev1.ConditionFalse) {
				return nil
			}

			return fmt.Errorf("expected migration to have condition %s as %v", migration.ClusterReadyCondition, corev1.ConditionTrue)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, creds)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Delete(ctx, vcsim)
		Expect(err).ToNot(HaveOccurred())
	})

})

var _ = Describe("verify vmware is errored", func() {
	var creds *corev1.Secret
	var vcsim *migration.VmwareSource

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
				Name:      "local",
				Namespace: "default",
			},
			Spec: migration.VmwareSourceSpec{
				EndpointAddress: "https://localhost/sdk",
				Datacenter:      "DC0",
				Credentials: corev1.SecretReference{
					Name:      creds.Name,
					Namespace: creds.Namespace,
				},
			},
		}

		err := k8sClient.Create(ctx, creds)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Create(ctx, vcsim)
		Expect(err).ToNot(HaveOccurred())
	})

	It("check vmware migration is ready", func() {
		// check status of migration object
		Eventually(func() error {
			vcsimObj := &migration.VmwareSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: vcsim.Name,
				Namespace: vcsim.Namespace}, vcsimObj)
			if err != nil {
				return err
			}

			if vcsimObj.Status.Status == "" {
				return nil
			}

			return fmt.Errorf("migration currently in state: %v, expected to be %s", vcsimObj.Status.Status, "")
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, creds)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Delete(ctx, vcsim)
		Expect(err).ToNot(HaveOccurred())
	})

})

var _ = Describe("verify vmware has invalid DC", func() {
	var creds *corev1.Secret
	var vcsim *migration.VmwareSource

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
				Name:      "local",
				Namespace: "default",
			},
			Spec: migration.VmwareSourceSpec{
				EndpointAddress: "",
				Datacenter:      "DC2",
				Credentials: corev1.SecretReference{
					Name:      creds.Name,
					Namespace: creds.Namespace,
				},
			},
		}

		err := k8sClient.Create(ctx, creds)
		Expect(err).ToNot(HaveOccurred())
		vcsim.Spec.EndpointAddress = fmt.Sprintf("https://localhost:%s/sdk", vcsimPort)
		err = k8sClient.Create(ctx, vcsim)
		Expect(err).ToNot(HaveOccurred())
	})

	It("check vmware migration is ready", func() {
		// check status of migration object
		Eventually(func() error {
			vcsimObj := &migration.VmwareSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: vcsim.Name,
				Namespace: vcsim.Namespace}, vcsimObj)
			if err != nil {
				return err
			}

			if vcsimObj.Status.Status == migration.ClusterNotReady {
				return nil
			}

			return fmt.Errorf("migration currently in state: %v, expected to be %s", vcsimObj.Status.Status, migration.ClusterNotReady)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		// check conditions on migration object
		Eventually(func() error {
			vcsimObj := &migration.VmwareSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: vcsim.Name,
				Namespace: vcsim.Namespace}, vcsimObj)
			if err != nil {
				return err
			}

			logrus.Info(vcsimObj.Status.Conditions)
			if util.ConditionExists(vcsimObj.Status.Conditions, migration.ClusterReadyCondition, corev1.ConditionFalse) &&
				util.ConditionExists(vcsimObj.Status.Conditions, migration.ClusterErrorCondition, corev1.ConditionTrue) {
				return nil
			}

			return fmt.Errorf("expected migration to have condition %s as %v", migration.ClusterErrorCondition, corev1.ConditionTrue)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, creds)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Delete(ctx, vcsim)
		Expect(err).ToNot(HaveOccurred())
	})

})
