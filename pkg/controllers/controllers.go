package controllers

import (
	"context"
	"time"

	harvester "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io"
	"github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	sc "github.com/harvester/vm-import-controller/pkg/controllers/migration"
	"github.com/harvester/vm-import-controller/pkg/crd"
	"github.com/harvester/vm-import-controller/pkg/generated/controllers/migration.harvesterhci.io"
	"github.com/rancher/lasso/pkg/cache"
	"github.com/rancher/lasso/pkg/client"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/pkg/generated/controllers/core"
	"github.com/rancher/wrangler/pkg/start"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/workqueue"
)

func Start(ctx context.Context, restConfig *rest.Config) error {
	if err := crd.Create(ctx, restConfig); err != nil {
		return err
	}

	if err := Register(ctx, restConfig); err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}

func Register(ctx context.Context, restConfig *rest.Config) error {
	rateLimit := workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 5*time.Minute)
	workqueue.DefaultControllerRateLimiter()
	clientFactory, err := client.NewSharedClientFactory(restConfig, nil)
	if err != nil {
		return err
	}

	cacheFactory := cache.NewSharedCachedFactory(clientFactory, nil)
	scf := controller.NewSharedControllerFactory(cacheFactory, &controller.SharedControllerFactoryOptions{
		DefaultRateLimiter: rateLimit,
		DefaultWorkers:     5,
	})

	if err != nil {
		return err
	}

	migrationFactory, err := migration.NewFactoryFromConfigWithOptions(restConfig, &migration.FactoryOptions{
		SharedControllerFactory: scf,
	})

	if err != nil {
		return err
	}

	coreFactory, err := core.NewFactoryFromConfigWithOptions(restConfig, &core.FactoryOptions{
		SharedControllerFactory: scf,
	})
	if err != nil {
		return err
	}

	harvesterFactory, err := harvester.NewFactoryFromConfigWithOptions(restConfig, &harvester.FactoryOptions{
		SharedControllerFactory: scf,
	})
	if err != nil {
		return err
	}

	kubevirtFactory, err := kubevirt.NewFactoryFromConfigWithOptions(restConfig, &kubevirt.FactoryOptions{
		SharedControllerFactory: scf,
	})
	if err != nil {
		return err
	}
	
	sc.RegisterVmareController(ctx, migrationFactory.Migration().V1beta1().VmwareSource(), coreFactory.Core().V1().Secret())
	sc.RegisterOpenstackController(ctx, migrationFactory.Migration().V1beta1().OpenstackSource(), coreFactory.Core().V1().Secret())

	sc.RegisterVMImportController(ctx, migrationFactory.Migration().V1beta1().VmwareSource(), migrationFactory.Migration().V1beta1().OpenstackSource(),
		coreFactory.Core().V1().Secret(), migrationFactory.Migration().V1beta1().VirtualMachineImport(),
		harvesterFactory.Harvesterhci().V1beta1().VirtualMachineImage(), kubevirtFactory.Kubevirt().V1().VirtualMachine(),
		coreFactory.Core().V1().PersistentVolumeClaim())

	return start.All(ctx, 1, migrationFactory)
}
