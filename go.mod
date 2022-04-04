module github.com/jijiechen/external-crd

go 1.16

require (
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	sigs.k8s.io/controller-runtime v0.10.0
	k8s.io/apiserver v0.23.1
)


replace (
	k8s.io/apiserver => github.com/clusternet/apiserver v0.0.0-20220224032722-ac3d780b913f
)
