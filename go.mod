module github.com/jijiechen/external-crd

go 1.16

require (
	github.com/clusternet/clusternet v0.8.0
	github.com/emicklei/go-restful v2.9.5+incompatible
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.17.0
	k8s.io/api v0.23.5
	k8s.io/apiextensions-apiserver v0.23.1
	k8s.io/apimachinery v0.23.5
	k8s.io/apiserver v0.23.5
	k8s.io/client-go v0.23.5
	k8s.io/code-generator v0.23.5
	k8s.io/controller-manager v0.23.5
	k8s.io/klog/v2 v2.30.0
	k8s.io/kube-aggregator v0.23.5
	k8s.io/metrics v0.23.5
	sigs.k8s.io/controller-runtime v0.10.0
	sigs.k8s.io/controller-tools v0.8.0
)

replace k8s.io/apiserver => github.com/clusternet/apiserver v0.0.0-20220224032722-ac3d780b913f
