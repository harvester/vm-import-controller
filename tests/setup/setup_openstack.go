package setup

import (
	"context"
	"fmt"
	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/source/openstack"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	openstackSecret         = "openstack-integration"
	openstackSourceCluster  = "openstack-integration"
	openstackKind           = "openstack"
	openstackVirtualMachine = "openstack-vm-export"
)

var (
	OpenstackSourceNamespacedName, OpenstackVMNamespacedName types.NamespacedName
)

// SetupOpenstack will try and setup a vmware migration based on GOVC environment variables
// It will check the following environment variables to build migration and importjob CRD's
// OS_AUTH_URL: Identify keystone endpoint
// OS_PROJECT_NAME: Project name where test instance is located
// OS_USERNAME: Username for migration secret
// OS_PASSWORD: Password for migration secret
// OS_USER_DOMAIN_NAME: domain name for user auth
// OS_VM_NAME: name of VM to be exported
// OS_REGION_NAME: OpenstackSource instance region to be used for testing
// SVC_ADDRESS: Exposes the local host as SVC url when creating VirtualDiskImage endpoints to download images from
func SetupOpenstack(ctx context.Context, k8sClient client.Client) error {
	OpenstackSourceNamespacedName = types.NamespacedName{
		Name:      openstackSourceCluster,
		Namespace: defaultNamespace,
	}

	OpenstackVMNamespacedName = types.NamespacedName{
		Name:      openstackVirtualMachine,
		Namespace: defaultNamespace,
	}
	fnList := []applyObject{
		setupOpenstackSecret,
		setupOpenstackSource,
		setupOpenstackVMExport,
	}

	for _, v := range fnList {
		if err := v(ctx, k8sClient); err != nil {
			return err
		}
	}

	return nil
}

func setupOpenstackSecret(ctx context.Context, k8sClient client.Client) error {
	s, err := openstack.SetupOpenstackSecretFromEnv(openstackSecret)
	if err != nil {
		return err
	}

	return k8sClient.Create(ctx, s)
}

func setupOpenstackSource(ctx context.Context, k8sClient client.Client) error {

	endpoint, region, err := openstack.SetupOpenstackSourceFromEnv()
	if err != nil {
		return err
	}

	s := &migration.OpenstackSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      openstackSourceCluster,
			Namespace: defaultNamespace,
		},
		Spec: migration.OpenstackSourceSpec{
			EndpointAddress: endpoint,
			Region:          region,
			Credentials: corev1.SecretReference{
				Name:      openstackSecret,
				Namespace: defaultNamespace,
			},
		},
	}

	return k8sClient.Create(ctx, s)
}

func setupOpenstackVMExport(ctx context.Context, k8sClient client.Client) error {
	vm, ok := os.LookupEnv("OS_VM_NAME")
	if !ok {
		return fmt.Errorf("env variable VM_NAME not specified")
	}

	_, ok = os.LookupEnv("SVC_ADDRESS")
	if !ok {
		return fmt.Errorf("env variable SVC_ADDRESS not specified")
	}

	j := &migration.VirtualMachineImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      openstackVirtualMachine,
			Namespace: defaultNamespace,
		},
		Spec: migration.VirtualMachineImportSpec{
			SourceCluster: corev1.ObjectReference{
				Name:       openstackSourceCluster,
				Namespace:  defaultNamespace,
				Kind:       openstackKind,
				APIVersion: defaultAPIVersion,
			},
			VirtualMachineName: vm,
		},
	}

	return k8sClient.Create(ctx, j)
}

func CleanupOpenstack(ctx context.Context, k8sClient client.Client) error {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      openstackSecret,
			Namespace: defaultNamespace,
		},
	}
	err := k8sClient.Delete(ctx, s)
	if err != nil {
		return err
	}

	vmware := &migration.OpenstackSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      openstackSourceCluster,
			Namespace: defaultNamespace,
		},
	}

	err = k8sClient.Delete(ctx, vmware)
	if err != nil {
		return err
	}

	i := &migration.VirtualMachineImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpenstackVMNamespacedName.Name,
			Namespace: defaultNamespace,
		},
	}

	return k8sClient.Delete(ctx, i)
}
