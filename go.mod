module github.com/jijiechen/external-crd

go 1.16

require (
	github.com/emicklei/go-restful v2.9.5+incompatible
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.17.0 // indirect
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.23.5
	k8s.io/apiextensions-apiserver v0.23.1
	k8s.io/apimachinery v0.23.5
	k8s.io/apiserver v0.23.5
	k8s.io/client-go v0.23.5
	k8s.io/code-generator v0.23.5
	k8s.io/component-base v0.23.5
	k8s.io/controller-manager v0.23.5
	k8s.io/klog/v2 v2.30.0
	k8s.io/kube-aggregator v0.23.5
	sigs.k8s.io/controller-tools v0.8.0
	sigs.k8s.io/yaml v1.3.0
)

// external-crd need this hack: https://github.com/clusternet/apimachinery/commit/6932fb9962a05e42686580c19ca052bd65c79ab9
replace k8s.io/apimachinery => github.com/clusternet/apimachinery v0.23.0-alpha.0.0.20220224022903-dc3dec363e8c