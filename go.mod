module github.com/harvester/vm-import-controller

go 1.25

toolchain go1.25.2

require (
	github.com/google/uuid v1.6.0
	github.com/gophercloud/gophercloud/v2 v2.5.0
	github.com/harvester/harvester v1.6.0
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.7.5
	github.com/longhorn/longhorn-manager v1.8.1
	github.com/onsi/ginkgo/v2 v2.23.3
	github.com/onsi/gomega v1.37.0
	github.com/ory/dockertest/v3 v3.9.1
	github.com/pkg/sftp v1.13.10
	github.com/rancher/lasso v0.2.3
	github.com/rancher/wrangler/v3 v3.2.2
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.10.0
	github.com/vmware/govmomi v0.52.0
	golang.org/x/crypto v0.41.0
	golang.org/x/sync v0.16.0
	k8s.io/api v0.33.1
	k8s.io/apiextensions-apiserver v0.33.1
	k8s.io/apimachinery v0.33.1
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/utils v0.0.0-20250604170112-4c0f3b243397
	kubevirt.io/api v1.5.0
	kubevirt.io/kubevirt v1.5.0
	libvirt.org/go/libvirtxml v1.11010.0
	sigs.k8s.io/cluster-api v1.9.4
	sigs.k8s.io/controller-runtime v0.20.2
)

require (
	dario.cat/mergo v1.0.1 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/c9s/goprocinfo v0.0.0-20210130143923-c95fcf8c64a8 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/continuity v0.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/cli v25.0.3+incompatible // indirect
	github.com/docker/docker v27.1.1+incompatible // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.12.2 // indirect
	github.com/evanphx/json-patch v5.9.11+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.8.0 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/zapr v1.3.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.1 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.1 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/gnostic-models v0.6.9 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20241210010833-40e02aabc2ad // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/gorilla/handlers v1.5.2 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/harvester/go-common v0.0.0-20250109132713-e748ce72a7ba // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.2.0 // indirect
	github.com/longhorn/go-common-libs v0.0.0-20250215052214-151615b29f8e // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/opencontainers/runc v1.1.14 // indirect
	github.com/openshift/api v0.0.0 // indirect
	github.com/openshift/client-go v3.9.0+incompatible // indirect
	github.com/openshift/custom-resource-status v1.1.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.78.2 // indirect
	github.com/prometheus/client_golang v1.22.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rancher/wrangler v1.1.2 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/shirou/gopsutil/v3 v3.24.5 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/mock v0.5.2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/exp v0.0.0-20250506013437-ce4c2cf36ca6 // indirect
	golang.org/x/mod v0.26.0 // indirect
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/term v0.34.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	golang.org/x/tools v0.35.0 // indirect
	golang.org/x/tools/go/packages/packagestest v0.1.1-deprecated // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gotest.tools/v3 v3.5.1 // indirect
	k8s.io/code-generator v0.33.1 // indirect
	k8s.io/gengo v0.0.0-20250130153323-76c5745d3511 // indirect
	k8s.io/gengo/v2 v2.0.0-20250207200755-1244d31929d7 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-aggregator v0.33.1 // indirect
	k8s.io/kube-openapi v0.31.9 // indirect
	kubevirt.io/client-go v1.5.0 // indirect
	kubevirt.io/containerized-data-importer-api v1.61.0 // indirect
	kubevirt.io/controller-lifecycle-operator-sdk/api v0.0.0-20220329064328-f3cc58c6ed90 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.7.0 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

replace (
	github.com/google/gnostic-models => github.com/google/gnostic-models v0.6.9
	github.com/openshift/api => github.com/openshift/api v0.0.0-20191219222812-2987a591a72c
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20200521150516-05eb9880269c
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring => github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.68.0
	github.com/rancher/rancher/pkg/apis => github.com/rancher/rancher/pkg/apis v0.0.0-20250828140533-07a90f09a491
	k8s.io/api => k8s.io/api v0.32.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.32.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.32.5
	k8s.io/apiserver => k8s.io/apiserver v0.32.5
	k8s.io/client-go => k8s.io/client-go v0.32.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.32.5
	k8s.io/code-generator => k8s.io/code-generator v0.31.5
	k8s.io/component-base => k8s.io/component-base v0.32.5
	k8s.io/gengo => k8s.io/gengo v0.0.0-20240826214909-a7b603a56eb7
	k8s.io/gengo/v2 => k8s.io/gengo/v2 v2.0.0-20240228010128-51d4e06bde70
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.32.5
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20240228011516-70dd3763d340
	kubevirt.io/api => kubevirt.io/api v1.6.0
	kubevirt.io/client-go => kubevirt.io/client-go v1.6.0
	kubevirt.io/kubevirt => kubevirt.io/kubevirt v1.6.0
)
