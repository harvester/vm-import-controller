package main

import (
	"log"
	"os"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/rancher/wrangler/v3/pkg/signals"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	kubevirtv1 "kubevirt.io/api/core/v1"

	source "github.com/harvester/vm-import-controller/pkg/apis/migration.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/controllers"
	"github.com/harvester/vm-import-controller/pkg/server"
)

func init() {
	var err error
	scheme := runtime.NewScheme()
	if err = source.AddToScheme(scheme); err != nil {
		log.Fatalf("failed to add source scheme, %v", err)
	}
	if err = harvesterv1beta1.AddToScheme(scheme); err != nil {
		log.Fatalf("failed to add harvesterv1beta1 scheme, %v", err)
	}
	if err = kubevirtv1.AddToScheme(scheme); err != nil {
		log.Fatalf("failed to add kubevirtv1 scheme, %v", err)
	}
	debug := os.Getenv("DEBUG")
	if debug == "true" || debug == "TRUE" {
		logrus.SetLevel(logrus.DebugLevel)
	}

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
