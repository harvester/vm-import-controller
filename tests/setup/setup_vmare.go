package setup

import (
	"context"
	"fmt"
	"os"

	importjob "github.com/harvester/vm-import-controller/pkg/apis/importjob.harvesterhci.io/v1beta1"
	source "github.com/harvester/vm-import-controller/pkg/apis/source.harvesterhci.io/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	secret            = "vmware-integration"
	sourceCluster     = "vmware-integration"
	virtualmachine    = "vm-export-test"
	defaultNamespace  = "default"
	defaultKind       = "vmware"
	defaultAPIVersion = "source.harvesterhci.io/v1beta1"
)

var (
	VmwareSourceNamespacedName, VmwareVMNamespacedName types.NamespacedName
)

type applyObject func(context.Context, client.Client) error

// SetupVMware will try and setup a vmware source based on GOVC environment variables
// It will check the following environment variables to build source and importjob CRD's
// GOVC_URL: Identify vsphere endpoint
// GOVC_DATACENTER: Identify vsphere datacenter
// GOVC_USERNAME: Username for source secret
// GOVC_PASSWORD: Password for source secret
// SVC_ADDRESS: local machine address, used to generate the URL that Harvester downloads the exported images from
// VM_NAME: name of VM to be exported
// VM_FOLDER: folder where VM pointed to by VM_NAME is located
func SetupVMware(ctx context.Context, k8sClient client.Client) error {
	VmwareSourceNamespacedName = types.NamespacedName{
		Name:      sourceCluster,
		Namespace: defaultNamespace,
	}

	VmwareVMNamespacedName = types.NamespacedName{
		Name:      virtualmachine,
		Namespace: defaultNamespace,
	}

	fnList := []applyObject{
		setupSecret,
		setupSource,
		setupVMExport,
	}

	for _, v := range fnList {
		if err := v(ctx, k8sClient); err != nil {
			return err
		}
	}

	return nil
}

func setupSecret(ctx context.Context, k8sClient client.Client) error {
	username, ok := os.LookupEnv("GOVC_USERNAME")
	if !ok {
		return fmt.Errorf("env variable GOVC_USERNAME not set")
	}
	password, ok := os.LookupEnv("GOVC_PASSWORD")
	if !ok {
		return fmt.Errorf("env variable GOVC_PASSWORD not set")
	}

	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret,
			Namespace: defaultNamespace,
		},
		StringData: map[string]string{
			"username": username,
			"password": password,
		},
	}

	return k8sClient.Create(ctx, s)
}

func setupSource(ctx context.Context, k8sClient client.Client) error {
	endpoint, ok := os.LookupEnv("GOVC_URL")
	if !ok {
		return fmt.Errorf("env variable GOVC_URL not set")
	}

	dc, ok := os.LookupEnv("GOVC_DATACENTER")
	if !ok {
		return fmt.Errorf("env variable GOVC_DATACENTER not set")
	}

	s := &source.Vmware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceCluster,
			Namespace: defaultNamespace,
		},
		Spec: source.VmwareClusterSpec{
			EndpointAddress: endpoint,
			Datacenter:      dc,
			Credentials: corev1.SecretReference{
				Name:      secret,
				Namespace: defaultNamespace,
			},
		},
	}

	return k8sClient.Create(ctx, s)

}

func setupVMExport(ctx context.Context, k8sClient client.Client) error {
	vm, ok := os.LookupEnv("VM_NAME")
	if !ok {
		return fmt.Errorf("env variable VM_NAME not specified")
	}

	_, ok = os.LookupEnv("SVC_ADDRESS")
	if !ok {
		return fmt.Errorf("env variable SVC_ADDRESS not specified")
	}

	folder, _ := os.LookupEnv("VM_FOLDER")

	j := &importjob.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtualmachine,
			Namespace: defaultNamespace,
		},
		Spec: importjob.VirtualMachineImportSpec{
			SourceCluster: corev1.ObjectReference{
				Name:       sourceCluster,
				Namespace:  defaultNamespace,
				Kind:       defaultKind,
				APIVersion: defaultAPIVersion,
			},
			VirtualMachineName: vm,
			Folder:             folder,
		},
	}

	return k8sClient.Create(ctx, j)
}

func CleanupVmware(ctx context.Context, k8sClient client.Client) error {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret,
			Namespace: defaultNamespace,
		},
	}
	err := k8sClient.Delete(ctx, s)
	if err != nil {
		return err
	}

	vmware := &source.Vmware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceCluster,
			Namespace: defaultNamespace,
		},
	}

	err = k8sClient.Delete(ctx, vmware)
	if err != nil {
		return err
	}

	i := &importjob.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtualmachine,
			Namespace: defaultNamespace,
		},
	}

	return k8sClient.Delete(ctx, i)
}
