package main

import (
	"log"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	source "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/controllers"
	"github.com/harvester/vm-import-controller/pkg/server"
	"github.com/rancher/wrangler/pkg/signals"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

func init() {
	scheme := runtime.NewScheme()
	source.AddToScheme(scheme)
	harvesterv1beta1.AddToScheme(scheme)
	kubevirtv1.AddToScheme(scheme)

}
func main() {

	ctx := signals.SetupSignalContext()

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		log.Fatal(err)
	}

	eg, egctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return controllers.Start(egctx, config)
	})

	eg.Go(func() error {
		return server.NewServer(egctx)
	})

	err = eg.Wait()
	if err != nil {
		log.Fatal(err)
	}
}
