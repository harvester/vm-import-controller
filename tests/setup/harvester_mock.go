package setup

import (
	"context"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util/crd"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"
	"kubevirt.io/kubevirt/pkg/virt-operator/resource/generate/components"
)

// InstallCRD will install the core harvester CRD's
// copied from harvester/harvester/pkg/data/crd.go
func InstallCRD(ctx context.Context, cfg *rest.Config) error {
	factory, err := crd.NewFactoryFromClient(ctx, cfg)
	if err != nil {
		return err
	}
	return factory.
		BatchCreateCRDsIfNotExisted(
			crd.NonNamespacedFromGV(harvesterv1.SchemeGroupVersion, "Setting", harvesterv1.Setting{}),
		).
		BatchCreateCRDsIfNotExisted(
			crd.FromGV(harvesterv1.SchemeGroupVersion, "KeyPair", harvesterv1.KeyPair{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "Upgrade", harvesterv1.Upgrade{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "Version", harvesterv1.Version{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineImage", harvesterv1.VirtualMachineImage{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineTemplate", harvesterv1.VirtualMachineTemplate{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineTemplateVersion", harvesterv1.VirtualMachineTemplateVersion{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineBackup", harvesterv1.VirtualMachineBackup{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "VirtualMachineRestore", harvesterv1.VirtualMachineRestore{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "Preference", harvesterv1.Preference{}),
			crd.FromGV(harvesterv1.SchemeGroupVersion, "SupportBundle", harvesterv1.SupportBundle{}),
		).
		BatchWait()
}

type kubeVirtCRDGenerator func() (*extv1.CustomResourceDefinition, error)

// GenerateKubeVirtCRD's will generate kubevirt CRDs
func GenerateKubeVirtCRD() ([]*extv1.CustomResourceDefinition, error) {
	v := []kubeVirtCRDGenerator{
		components.NewVirtualMachineCrd,
		components.NewVirtualMachineInstanceCrd,
		components.NewPresetCrd,
		components.NewReplicaSetCrd,
		components.NewVirtualMachineInstanceMigrationCrd,
		components.NewVirtualMachinePoolCrd,
		components.NewVirtualMachineSnapshotCrd,
		components.NewVirtualMachineSnapshotContentCrd,
		components.NewVirtualMachineRestoreCrd,
		components.NewVirtualMachineFlavorCrd,
		components.NewVirtualMachineClusterFlavorCrd,
		components.NewMigrationPolicyCrd,
	}

	var result []*extv1.CustomResourceDefinition
	for _, m := range v {
		crdList, err := m()
		if err != nil {
			return nil, err
		}
		result = append(result, crdList)
	}

	return result, nil
}
