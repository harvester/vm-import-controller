package setup

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
)

const (
	kvmSecret            = "kvm-integration"
	kvmSourceCluster     = "kvm-integration"
	kvmVirtualMachine    = "kvm-export-test"
	kvmDefaultNamespace  = "default"
	kvmDefaultKind       = "KVMSource"
	kvmDefaultAPIVersion = "migration.harvesterhci.io/v1beta1"
)

var (
	KVMSourceNamespacedName types.NamespacedName
	KVMVMNamespacedName     types.NamespacedName
)

// SetupKVM will try and set up a KVM migration based on environment variables.
// It will check the following environment variables to build migration and importjob CRD's
// KVM_LIBVIRT_URI: Identify libvirt endpoint (e.g., qemu+ssh://user@host/system)
// KVM_SSH_USER: Username for migration secret
// KVM_SSH_PASSWORD: Password for migration secret
// SVC_ADDRESS: local machine address, used to generate the URL that Harvester downloads the exported images from
// VM_NAME: name of VM to be exported
func SetupKVM(ctx context.Context, k8sClient client.Client) error {
	KVMSourceNamespacedName = types.NamespacedName{
		Name:      kvmSourceCluster,
		Namespace: kvmDefaultNamespace,
	}

	KVMVMNamespacedName = types.NamespacedName{
		Name:      kvmVirtualMachine,
		Namespace: kvmDefaultNamespace,
	}

	fnList := []applyObject{
		setupKVMSecret,
		setupKVMSource,
		setupKVMVMExport,
	}

	for _, v := range fnList {
		if err := v(ctx, k8sClient); err != nil {
			return err
		}
	}

	return nil
}

func setupKVMSecret(ctx context.Context, k8sClient client.Client) error {
	username, ok := os.LookupEnv("KVM_SSH_USER")
	if !ok {
		return fmt.Errorf("env variable KVM_SSH_USER not set")
	}

	stringData := map[string]string{
		"username": username,
	}

	password, hasPassword := os.LookupEnv("KVM_SSH_PASSWORD")
	if hasPassword {
		stringData["password"] = password
	}

	keyPath, hasKey := os.LookupEnv("KVM_SSH_PRIVATE_KEY_PATH")
	if hasKey {
		keyContent, err := os.ReadFile(keyPath)
		if err != nil {
			return fmt.Errorf("failed to read private key file %s: %v", keyPath, err)
		}
		stringData["privateKey"] = string(keyContent)
	}

	if !hasPassword && !hasKey {
		return fmt.Errorf("neither KVM_SSH_PASSWORD nor KVM_SSH_PRIVATE_KEY_PATH set")
	}

	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kvmSecret,
			Namespace: kvmDefaultNamespace,
		},
		StringData: stringData,
	}

	return k8sClient.Create(ctx, s)
}

func setupKVMSource(ctx context.Context, k8sClient client.Client) error {
	endpoint, ok := os.LookupEnv("KVM_URL")
	if !ok {
		return fmt.Errorf("env variable KVM_URL not set")
	}

	s := &migration.KVMSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kvmSourceCluster,
			Namespace: kvmDefaultNamespace,
		},
		Spec: migration.KVMSourceSpec{
			EndpointAddress: endpoint,
			Credentials: corev1.SecretReference{
				Name:      kvmSecret,
				Namespace: kvmDefaultNamespace,
			},
		},
	}

	return k8sClient.Create(ctx, s)

}

func setupKVMVMExport(ctx context.Context, k8sClient client.Client) error {
	vm, ok := os.LookupEnv("VM_NAME")
	if !ok {
		return fmt.Errorf("env variable VM_NAME not specified")
	}

	_, ok = os.LookupEnv("SVC_ADDRESS")
	if !ok {
		return fmt.Errorf("env variable SVC_ADDRESS not specified")
	}

	j := &migration.VirtualMachineImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kvmVirtualMachine,
			Namespace: kvmDefaultNamespace,
		},
		Spec: migration.VirtualMachineImportSpec{
			SourceCluster: corev1.ObjectReference{
				Name:       kvmSourceCluster,
				Namespace:  kvmDefaultNamespace,
				Kind:       kvmDefaultKind,
				APIVersion: kvmDefaultAPIVersion,
			},
			VirtualMachineName: vm,
		},
	}

	return k8sClient.Create(ctx, j)
}

func CleanupKVM(ctx context.Context, k8sClient client.Client) error {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kvmSecret,
			Namespace: kvmDefaultNamespace,
		},
	}
	err := k8sClient.Delete(ctx, s)
	if err != nil {
		return err
	}

	kvm := &migration.KVMSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kvmSourceCluster,
			Namespace: kvmDefaultNamespace,
		},
	}

	err = k8sClient.Delete(ctx, kvm)
	if err != nil {
		return err
	}

	i := &migration.VirtualMachineImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kvmVirtualMachine,
			Namespace: kvmDefaultNamespace,
		},
	}

	return k8sClient.Delete(ctx, i)
}
