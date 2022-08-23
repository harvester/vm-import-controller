package integration

import (
	"context"
	"os"
	"testing"
	"time"

	importjob "github.com/harvester/vm-import-controller/pkg/apis/importjob.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/server"

	"github.com/harvester/vm-import-controller/tests/setup"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	source "github.com/harvester/vm-import-controller/pkg/apis/source.harvesterhci.io/v1beta1"
	"github.com/harvester/vm-import-controller/pkg/controllers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/ory/dockertest/v3"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	log "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	ctx, egctx  context.Context
	k8sClient   client.Client
	testEnv     *envtest.Environment
	cancel      context.CancelFunc
	scheme      = runtime.NewScheme()
	eg          *errgroup.Group
	pool        *dockertest.Pool
	vcsimPort   string
	vcsimMock   *dockertest.Resource
	useExisting bool
)

func TestIntegration(t *testing.T) {
	defer GinkgoRecover()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.TODO())

	existing, ok := os.LookupEnv("USE_EXISTING_CLUSTER")
	if ok && existing == "true" {
		useExisting = true
	}

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{}

	if !useExisting {
		crds, err := setup.GenerateKubeVirtCRD()
		Expect(err).ToNot(HaveOccurred())
		testEnv.CRDInstallOptions = envtest.CRDInstallOptions{
			CRDs: crds,
		}
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = source.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = apiextensions.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = corev1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = harvesterv1beta1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = kubevirtv1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = importjob.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	err = setup.InstallCRD(ctx, cfg)
	Expect(err).NotTo(HaveOccurred())

	eg, egctx = errgroup.WithContext(ctx)
	eg.Go(func() error {
		return controllers.Start(egctx, cfg)
	})

	eg.Go(func() error {
		return server.NewServer(egctx)
	})

	eg.Go(func() error {
		return eg.Wait()
	})

	pool, err = dockertest.NewPool("")
	Expect(err).NotTo(HaveOccurred())
	runOpts := &dockertest.RunOptions{
		Name:       "vcsim-integration",
		Repository: "vmware/vcsim",
		Tag:        "v0.29.0",
	}

	vcsimMock, err = pool.RunWithOptions(runOpts)
	Expect(err).NotTo(HaveOccurred())

	vcsimPort = vcsimMock.GetPort("8989/tcp")

	time.Sleep(30 * time.Second)

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	if vcsimMock != nil {
		err := pool.Purge(vcsimMock)
		Expect(err).NotTo(HaveOccurred())
	}
	egctx.Done()
	cancel()
	testEnv.Stop()
})
