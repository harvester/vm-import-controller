package crd

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/crd"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	migration "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
)

func List() []crd.CRD {
	return []crd.CRD{
		newCRD("migration.harvesterhci.io", &migration.VmwareSource{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Status", ".status.status")
		}),
		newCRD("migration.harvesterhci.io", &migration.OvaSource{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Status", ".status.status")
		}),
		newCRD("migration.harvesterhci.io", &migration.OpenstackSource{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Status", ".status.status")
		}),
		newCRD("migration.harvesterhci.io", &migration.VirtualMachineImport{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Status", ".status.importStatus")
		}),
	}
}

func Create(ctx context.Context, cfg *rest.Config) error {
	factory, err := crd.NewFactoryFromClient(cfg)
	if err != nil {
		return err
	}

	return factory.BatchCreateCRDs(ctx, List()...).BatchWait()
}

func newCRD(group string, obj interface{}, customize func(crd.CRD) crd.CRD) crd.CRD {
	crd := crd.CRD{
		GVK: schema.GroupVersionKind{
			Group:   group,
			Version: "v1beta1",
		},
		Status:       true,
		NonNamespace: false,
		SchemaObject: obj,
	}
	if customize != nil {
		crd = customize(crd)
	}
	return crd
}
