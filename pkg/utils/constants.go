/*
Copyright 2022 Jijie Chen.
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

package utils

import "time"

const (
	// KcrdSystemNamespace is the default system namespace where we place system components.
	// This could be re-configured with flag "--leader-elect-resource-namespace"
	KcrdSystemNamespace = "external-crd-system"

	// KcrdReservedNamespace is the default namespace to store Manifest into
	KcrdReservedNamespace = "external-crd-reserved"
)

const (
	AppFinalizer string = "k8s.jijiechen.com/finalizer"
)

const (
	// DefaultResync means the default resync time
	DefaultResync = time.Hour * 12
)

const (
	Category = "external-crd"
)

// label key
const (
	ObjectCreatedByLabel = "k8s.jijiechen.com/created-by"

	// the source info where this object belongs to or controlled by
	ConfigGroupLabel     = "k8s.jijiechen.com/config.group"
	ConfigVersionLabel   = "k8s.jijiechen.com/config.version"
	ConfigKindLabel      = "k8s.jijiechen.com/config.kind"
	ConfigNameLabel      = "k8s.jijiechen.com/config.name"
	ConfigNamespaceLabel = "k8s.jijiechen.com/config.namespace"
)

// label value
const (
	CredentialsAuto = "credentials-auto"
	RBACDefaults    = "rbac-defaults"

	ExternalCrdAppName = "external-crd"
)
