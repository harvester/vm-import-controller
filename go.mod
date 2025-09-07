module github.com/harvester/vm-import-controller

go 1.21

require (
	github.com/google/uuid v1.3.0
	github.com/gophercloud/gophercloud v0.7.0
	github.com/harvester/harvester v1.1.0-rc2
	github.com/onsi/ginkgo/v2 v2.1.4
	github.com/onsi/gomega v1.19.0
	github.com/ory/dockertest/v3 v3.9.1
	github.com/rancher/lasso v0.0.0-20221227210133-6ea88ca2fbcc
	github.com/rancher/wrangler v1.1.0
	github.com/sirupsen/logrus v1.9.0
	github.com/stretchr/testify v1.8.1
	github.com/vmware/govmomi v0.29.0
	golang.org/x/sync v0.3.0
	k8s.io/api v0.25.4
	k8s.io/apiextensions-apiserver v0.25.4
	k8s.io/apimachinery v0.25.4
	k8s.io/client-go v12.0.0+incompatible
	kubevirt.io/api v0.54.0
	kubevirt.io/kubevirt v0.54.0
	sigs.k8s.io/controller-runtime v0.12.2
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.1.3 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/containerd/continuity v0.3.0 // indirect
	github.com/coreos/prometheus-operator v0.38.1-0.20200424145508-7e176fda06cc // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/cli v20.10.14+incompatible // indirect
	github.com/docker/docker v20.10.12+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/emicklei/go-restful v2.15.0+incompatible // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-kit/kit v0.10.0 // indirect
	github.com/go-logfmt/logfmt v0.5.0 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/zapr v1.2.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.6 // indirect
	github.com/go-openapi/swag v0.21.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.0.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200331171230-d50e42f2b669 // indirect
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.2.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/mapstructure v1.4.3 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.3-0.20211202183452-c5a74bcca799 // indirect
	github.com/opencontainers/runc v1.1.2 // indirect
	github.com/openshift/api v0.0.0 // indirect
	github.com/openshift/client-go v0.0.0 // indirect
	github.com/openshift/custom-resource-status v1.1.2 // indirect
	github.com/pborman/uuid v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.12.1 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/rogpeppe/go-internal v1.8.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190809123943-df4f5c81cb3b // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.19.1 // indirect
	golang.org/x/mod v0.13.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/oauth2 v0.13.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/term v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.13.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto v0.0.0-20220502173005-c8bf987b8c21 // indirect
	google.golang.org/grpc v1.47.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/code-generator v0.25.4 // indirect
	k8s.io/gengo v0.0.0-20211129171323-c02415ce4185 // indirect
	k8s.io/klog/v2 v2.70.1 // indirect
	k8s.io/kube-aggregator v0.25.4 // indirect
	k8s.io/kube-openapi v0.0.0-20220803162953-67bda5d908f1 // indirect
	k8s.io/utils v0.0.0-20220728103510-ee6ede2d64ed // indirect
	kubevirt.io/client-go v0.49.0 // indirect
	kubevirt.io/containerized-data-importer-api v1.50.0 // indirect
	kubevirt.io/controller-lifecycle-operator-sdk/api v0.0.0-20220329064328-f3cc58c6ed90 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	github.com/containerd/containerd => github.com/containerd/containerd v1.6.18
	github.com/docker/distribution => github.com/docker/distribution v2.8.0+incompatible
	github.com/emicklei/go-restful => github.com/emicklei/go-restful v2.16.0+incompatible
	github.com/openshift/api => github.com/openshift/api v0.0.0-20191219222812-2987a591a72c
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20200521150516-05eb9880269c
	github.com/operator-framework/operator-lifecycle-manager => github.com/operator-framework/operator-lifecycle-manager v0.0.0-20190128024246-5eb7ae5bdb7a
	github.com/rancher/rancher/pkg/apis => github.com/rancher/rancher/pkg/apis v0.0.0-20211208233239-77392a65423d
	github.com/rancher/rancher/pkg/client => github.com/rancher/rancher/pkg/client v0.0.0-20211208233239-77392a65423d
	golang.org/x/net => golang.org/x/net v0.17.0
	golang.org/x/text => golang.org/x/text v0.3.8
	k8s.io/api => k8s.io/api v0.23.7 // Dropped to v0.20.2 to handle kubevirt deps for installing CRDs
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.23.7
	k8s.io/apimachinery => k8s.io/apimachinery v0.23.7
	k8s.io/apiserver => k8s.io/apiserver v0.23.7
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.23.7
	k8s.io/client-go => k8s.io/client-go v0.23.7 // Dropped to v0.20.2 to handle kubevirt deps for installing CRDs
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.23.7
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.23.7
	k8s.io/code-generator => k8s.io/code-generator v0.23.7
	k8s.io/component-base => k8s.io/component-base v0.23.7
	k8s.io/component-helpers => k8s.io/component-helpers v0.23.7
	k8s.io/controller-manager => k8s.io/controller-manager v0.23.7
	k8s.io/cri-api => k8s.io/cri-api v0.23.7
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.23.7
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.23.7
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.23.7
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20220124234850-424119656bbf
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.23.7
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.23.7
	k8s.io/kubectl => k8s.io/kubectl v0.23.7
	k8s.io/kubelet => k8s.io/kubelet v0.23.7
	k8s.io/kubernetes => k8s.io/kubernetes v1.23.7
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.23.7
	k8s.io/metrics => k8s.io/metrics v0.23.7
	k8s.io/mount-utils => k8s.io/mount-utils v0.23.7
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.23.7
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.23.7
	kubevirt.io/api => github.com/kubevirt/api v0.59.2
	kubevirt.io/client-go => github.com/kubevirt/client-go v0.55.1
	kubevirt.io/containerized-data-importer-api => kubevirt.io/containerized-data-importer-api v1.47.0
	kubevirt.io/kubevirt => kubevirt.io/kubevirt v0.55.1
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2
)
