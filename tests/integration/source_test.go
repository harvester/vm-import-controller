package integration

import (
	"fmt"

	"github.com/harvester/vm-import-controller/pkg/source/openstack"

	source "github.com/harvester/vm-import-controller/pkg/apis/source.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("verify vmware is ready", func() {
	var creds *corev1.Secret
	var vcsim *source.Vmware

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

		vcsim = &source.Vmware{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local",
				Namespace: "default",
			},
			Spec: source.VmwareClusterSpec{
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

	It("check vmware source is ready", func() {
		// check status of source object
		Eventually(func() error {
			vcsimObj := &source.Vmware{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: vcsim.Name,
				Namespace: vcsim.Namespace}, vcsimObj)
			if err != nil {
				return err
			}

			if vcsimObj.Status.Status == source.ClusterReady {
				return nil
			}

			return fmt.Errorf("source currently in state: %v, expected to be %s", vcsimObj.Status.Status, source.ClusterReady)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		// check conditions on source object
		Eventually(func() error {
			vcsimObj := &source.Vmware{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: vcsim.Name,
				Namespace: vcsim.Namespace}, vcsimObj)
			if err != nil {
				return err
			}
			if util.ConditionExists(vcsimObj.Status.Conditions, source.ClusterReadyCondition, corev1.ConditionTrue) &&
				util.ConditionExists(vcsimObj.Status.Conditions, source.ClusterErrorCondition, corev1.ConditionFalse) {
				return nil
			}

			return fmt.Errorf("expected source to have condition %s as %v", source.ClusterReadyCondition, corev1.ConditionTrue)
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
	var vcsim *source.Vmware

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

		vcsim = &source.Vmware{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local",
				Namespace: "default",
			},
			Spec: source.VmwareClusterSpec{
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

	It("check vmware source is ready", func() {
		// check status of source object
		Eventually(func() error {
			vcsimObj := &source.Vmware{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: vcsim.Name,
				Namespace: vcsim.Namespace}, vcsimObj)
			if err != nil {
				return err
			}

			if vcsimObj.Status.Status == "" {
				return nil
			}

			return fmt.Errorf("source currently in state: %v, expected to be %s", vcsimObj.Status.Status, "")
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
	var vcsim *source.Vmware

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

		vcsim = &source.Vmware{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local",
				Namespace: "default",
			},
			Spec: source.VmwareClusterSpec{
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

	It("check vmware source is ready", func() {
		// check status of source object
		Eventually(func() error {
			vcsimObj := &source.Vmware{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: vcsim.Name,
				Namespace: vcsim.Namespace}, vcsimObj)
			if err != nil {
				return err
			}

			if vcsimObj.Status.Status == source.ClusterNotReady {
				return nil
			}

			return fmt.Errorf("source currently in state: %v, expected to be %s", vcsimObj.Status.Status, source.ClusterNotReady)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		// check conditions on source object
		Eventually(func() error {
			vcsimObj := &source.Vmware{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: vcsim.Name,
				Namespace: vcsim.Namespace}, vcsimObj)
			if err != nil {
				return err
			}

			logrus.Info(vcsimObj.Status.Conditions)
			if util.ConditionExists(vcsimObj.Status.Conditions, source.ClusterReadyCondition, corev1.ConditionFalse) &&
				util.ConditionExists(vcsimObj.Status.Conditions, source.ClusterErrorCondition, corev1.ConditionTrue) {
				return nil
			}

			return fmt.Errorf("expected source to have condition %s as %v", source.ClusterErrorCondition, corev1.ConditionTrue)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, creds)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Delete(ctx, vcsim)
		Expect(err).ToNot(HaveOccurred())
	})

})

var _ = Describe("verify openstack is ready", func() {
	var creds *corev1.Secret
	var o *source.Openstack

	const secretName = "devstack"
	BeforeEach(func() {
		var err error
		creds, err = openstack.SetupOpenstackSecretFromEnv(secretName)
		Expect(err).ToNot(HaveOccurred())
		endpoint, region, err := openstack.SetupOpenstackSourceFromEnv()
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Create(ctx, creds)
		Expect(err).ToNot(HaveOccurred())

		o = &source.Openstack{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "devstack",
				Namespace: "default",
			},
			Spec: source.OpenstackSpec{
				EndpointAddress: endpoint,
				Region:          region,
				Credentials: corev1.SecretReference{
					Name:      secretName,
					Namespace: "default",
				},
			},
		}
		err = k8sClient.Create(ctx, o)
		Expect(err).ToNot(HaveOccurred())
	})

	It("check openstack source is ready", func() {
		Eventually(func() error {
			oObj := &source.Openstack{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: o.Name, Namespace: o.Namespace}, oObj)
			if err != nil {
				return err
			}
			if oObj.Status.Status == source.ClusterReady {
				return nil
			}
			return fmt.Errorf("source currently in state: %v, expected to be %s", oObj.Status.Status, source.ClusterReady)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		// check conditions on source object
		Eventually(func() error {
			oObj := &source.Openstack{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: o.Name,
				Namespace: o.Namespace}, oObj)
			if err != nil {
				return err
			}
			if util.ConditionExists(oObj.Status.Conditions, source.ClusterReadyCondition, corev1.ConditionTrue) &&
				util.ConditionExists(oObj.Status.Conditions, source.ClusterErrorCondition, corev1.ConditionFalse) {
				return nil
			}

			return fmt.Errorf("expected source to have condition %s as %v", source.ClusterReadyCondition, corev1.ConditionTrue)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})
	AfterEach(func() {
		err := k8sClient.Delete(ctx, creds)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Delete(ctx, o)
		Expect(err).ToNot(HaveOccurred())

	})
})
