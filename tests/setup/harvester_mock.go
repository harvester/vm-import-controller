package setup

import (
	"context"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"kubevirt.io/kubevirt/pkg/virt-operator/resource/generate/components"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util/crd"
)

// InstallCRD will install the core Harvester CRD's
// partly copied from harvester/harvester/pkg/data/crd.go
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

// GenerateCRD will generate other required CRD's
func GenerateCRD() ([]*extv1.CustomResourceDefinition, error) {
	kubeVirtCrds, err := generateKubeVirtCRD()
	if err != nil {
		return nil, err
	}
	k8sCniCncfIoCrds, err := generateK8sCniCncfIoCRD()
	if err != nil {
		return nil, err
	}
	return append(kubeVirtCrds, k8sCniCncfIoCrds...), nil
}

type kubeVirtCRDGenerator func() (*extv1.CustomResourceDefinition, error)

// generateKubeVirtCRD will generate kubevirt CRDs
func generateKubeVirtCRD() ([]*extv1.CustomResourceDefinition, error) {
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
		components.NewMigrationPolicyCrd,
	}

	results := []*extv1.CustomResourceDefinition{}
	for _, m := range v {
		crdList, err := m()
		if err != nil {
			return nil, err
		}
		results = append(results, crdList)
	}

	return results, nil
}

// generateK8sCniCncfIoCRD will generate k8s.cni.cncf.io CRDs
func generateK8sCniCncfIoCRD() ([]*extv1.CustomResourceDefinition, error) {
	var results []*extv1.CustomResourceDefinition
	results = append(results,
		// See https://github.com/k8snetworkplumbingwg/network-attachment-definition-client/blob/v1.3.0/artifacts/networks-crd.yaml
		&extv1.CustomResourceDefinition{
			TypeMeta: metav1.TypeMeta{
				APIVersion: extv1.SchemeGroupVersion.String(),
				Kind:       "CustomResourceDefinition",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "network-attachment-definitions.k8s.cni.cncf.io",
			},
			Spec: extv1.CustomResourceDefinitionSpec{
				Group: "k8s.cni.cncf.io",
				Scope: "Namespaced",
				Names: extv1.CustomResourceDefinitionNames{
					Plural:     "network-attachment-definitions",
					Singular:   "network-attachment-definition",
					Kind:       "NetworkAttachmentDefinition",
					ShortNames: []string{"net-attach-def"},
				},
				Versions: []extv1.CustomResourceDefinitionVersion{
					{
						Name:    "v1",
						Served:  true,
						Storage: true,
						Schema: &extv1.CustomResourceValidation{
							OpenAPIV3Schema: &extv1.JSONSchemaProps{
								Type: "object",
								Properties: map[string]extv1.JSONSchemaProps{
									"spec": {
										Type: "object",
										Properties: map[string]extv1.JSONSchemaProps{
											"config": {
												Type: "string",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})
	return results, nil
}
