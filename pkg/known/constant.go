/*
Copyright 2021 The Clusternet Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package known

const (
	// NamePrefixForClusternetObjects is a prefix name for generating external-crd related objects for child cluster,
	// such as namespace, sa, etc
	NamePrefixForExternalCrdObjects = "kcrd-"

	// ClusterAPIServerURLKey denotes the apiserver address
	ClusterAPIServerURLKey = "apiserver-advertise-url"

	// KcrdSystemNamespace is the default system namespace where we place system components.
	// This could be re-configured with flag "--leader-elect-resource-namespace"
	KcrdSystemNamespace = "external-crd-system"

	// KcrdReservedNamespace is the default namespace to store Manifest into
	KcrdReservedNamespace = "external-crd-reserved"
)

const (
	AppFinalizer string = "k8s.jijiechen.com/finalizer"
)

// fields should be ignored when compared
const (
	MetaGeneration    = "/metadata/generation"
	CreationTimestamp = "/metadata/creationTimestamp"
	ManagedFields     = "/metadata/managedFields"
	MetaUID           = "/metadata/uid"
	MetaSelflink      = "/metadata/selfLink"
)

const (
	Category = "external-crd.overlay"
)

const (
	// NoteLengthLimit denotes the maximum note length.
	// copied from k8s.io/kubernetes/pkg/apis/core/validation/events.go
	NoteLengthLimit = 1024
)
