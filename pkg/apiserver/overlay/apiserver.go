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

package apiserver

import (
	"fmt"
	"github.com/jijiechen/external-crd/pkg/crdmanifests"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/cache"
	"path"
	"strings"
	"time"

	overlayinstall "github.com/jijiechen/external-crd/pkg/apis/overlay/install"
	overlayapi "github.com/jijiechen/external-crd/pkg/apis/overlay/v1alpha1"
	kcrd "github.com/jijiechen/external-crd/pkg/generated/clientset/versioned"
	kcrdlisters "github.com/jijiechen/external-crd/pkg/generated/listers/kcrd/v1alpha1"
	autoscalingapiv1 "k8s.io/api/autoscaling/v1"
	crdinformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	apiextensionsv1lister "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	genericapi "k8s.io/apiserver/pkg/endpoints"
	genericdiscovery "k8s.io/apiserver/pkg/endpoints/discovery"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/storageversion"
	"k8s.io/client-go/discovery"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	apiservicelisters "k8s.io/kube-aggregator/pkg/client/listers/apiregistration/v1"
)

var (
	// Scheme defines methods for serializing and deserializing API objects.
	Scheme = runtime.NewScheme()
	// Codecs provides methods for retrieving codecs and serializers for specific
	// versions and content types.
	Codecs = serializer.NewCodecFactory(Scheme)
	// ParameterCodec handles versioning of objects that are converted to query parameters.
	ParameterCodec = runtime.NewParameterCodec(Scheme)
)

const (
	kcrdGroupSuffix = ".jijiechen.com"
)

func init() {
	overlayinstall.Install(Scheme)

	// we need to add the options to empty v1
	// TODO fix the server code to avoid this
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})
	metav1.AddToGroupVersion(Scheme, metav1.SchemeGroupVersion)

	// TODO: keep the generic API server from wanting this
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
		&metav1.List{},
	)

	Scheme.AddUnversionedTypes(autoscalingapiv1.SchemeGroupVersion,
		&autoscalingapiv1.Scale{},
	)
}

// OverlayAPIServer will make a overlay copy for all the APIs
type OverlayAPIServer struct {
	GenericAPIServer    *genericapiserver.GenericAPIServer
	maxRequestBodyBytes int64
	minRequestTimeout   int

	// admissionControl performs deep inspection of a given request (including content)
	// to set values and determine whether its allowed
	admissionControl admission.Interface

	kubeRESTClient restclient.Interface
	kcrdClient     *kcrd.Clientset

	kcrdLister       kcrdlisters.KubernetesCrdLister
	crdLister        apiextensionsv1lister.CustomResourceDefinitionLister
	crdSynced        cache.InformerSynced
	crdHandler       *crdHandler
	apiserviceLister apiservicelisters.APIServiceLister

	// namespace where Manifests are created
	reservedNamespace string
}

func NewOverlayAPIServer(apiserver *genericapiserver.GenericAPIServer, maxRequestBodyBytes int64, minRequestTimeout int,
	admissionControl admission.Interface,
	kubeRESTClient restclient.Interface, kcrdClient *kcrd.Clientset, manifestLister kcrdlisters.KubernetesCrdLister,
	apiserviceLister apiservicelisters.APIServiceLister, crdInformerFactory crdinformers.SharedInformerFactory,
	reservedNamespace string) *OverlayAPIServer {

	return &OverlayAPIServer{
		GenericAPIServer:    apiserver,
		maxRequestBodyBytes: maxRequestBodyBytes,
		minRequestTimeout:   minRequestTimeout,
		admissionControl:    admissionControl,
		kubeRESTClient:      kubeRESTClient,
		kcrdClient:          kcrdClient,
		kcrdLister:          manifestLister,
		crdLister:           crdInformerFactory.Apiextensions().V1().CustomResourceDefinitions().Lister(),
		crdSynced:           crdInformerFactory.Apiextensions().V1().CustomResourceDefinitions().Informer().HasSynced,
		crdHandler: NewCRDHandler(
			kubeRESTClient, kcrdClient, manifestLister, apiserviceLister,
			crdInformerFactory.Apiextensions().V1().CustomResourceDefinitions(),
			minRequestTimeout, maxRequestBodyBytes, admissionControl, apiserver.Authorizer, apiserver.Serializer, reservedNamespace),
		apiserviceLister:  apiserviceLister,
		reservedNamespace: reservedNamespace,
	}
}

func (ols *OverlayAPIServer) InstallOverlayAPIGroups(stopCh <-chan struct{}, cl discovery.DiscoveryInterface) error {
	// Wait for all CRDs to sync before installing overlay api resources.
	klog.V(5).Info("overlay apiserver is waiting for informer caches to sync")

	cache.WaitForCacheSync(stopCh, ols.crdSynced)
	crds, err := ols.crdLister.List(labels.Everything())
	if err != nil {
		return err
	}
	crdGroups := sets.String{}
	for _, crd := range crds {
		if crdGroups.Has(crd.Spec.Group) {
			continue
		}
		crdGroups = crdGroups.Insert(crd.Spec.Group)
	}

	apiGroupResources, err := restmapper.GetAPIGroupResources(cl)
	if err != nil {
		return err
	}

	overlayv1alpha1storage := map[string]rest.Storage{}
	nsRESTSet := false
	for _, apiGroupResource := range apiGroupResources {
		if apiGroupResource.Group.Name != "" {
			continue
		}

		for _, apiresource := range normalizeAPIGroupResources(apiGroupResource) {
			if apiresource.Name == "namespaces" {
				nsRESTSet = true

				Scheme.AddKnownTypeWithName(schema.GroupVersion{Group: apiGroupResource.Group.Name,
					Version: apiresource.Version}.WithKind(apiresource.Kind), &unstructured.Unstructured{})

				resourceRest := crdmanifests.NewREST(ols.kubeRESTClient, ols.kcrdClient, ParameterCodec, ols.kcrdLister, ols.reservedNamespace)
				resourceRest.SetNamespaceScoped(apiresource.Namespaced)
				resourceRest.SetName(apiresource.Name)
				resourceRest.SetShortNames(apiresource.ShortNames)
				resourceRest.SetKind(apiresource.Kind)
				resourceRest.SetGroup(apiresource.Group)
				resourceRest.SetVersion(apiresource.Version)
				overlayv1alpha1storage[apiresource.Name] = resourceRest
				break
			}
		}

		if nsRESTSet {
			break
		}
	}

	overlayAPIGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(overlayapi.GroupName, Scheme, ParameterCodec, Codecs)
	overlayAPIGroupInfo.PrioritizedVersions = []schema.GroupVersion{
		{
			Group:   overlayapi.GroupName,
			Version: overlayapi.SchemeGroupVersion.Version,
		},
	}
	overlayAPIGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = overlayv1alpha1storage
	return ols.installAPIGroups(&overlayAPIGroupInfo)
}

// Exposes given api groups in the API.
// copied from k8s.io/apiserver/pkg/server/genericapiserver.go and modified
func (ols *OverlayAPIServer) installAPIGroups(apiGroupInfos ...*genericapiserver.APIGroupInfo) error {
	for _, apiGroupInfo := range apiGroupInfos {
		// Do not register empty group or empty version.  Doing so claims /apis/ for the wrong entity to be returned.
		// Catching these here places the error  much closer to its origin
		if len(apiGroupInfo.PrioritizedVersions[0].Group) == 0 {
			return fmt.Errorf("cannot register handler with an empty group for %#v", *apiGroupInfo)
		}
		if len(apiGroupInfo.PrioritizedVersions[0].Version) == 0 {
			return fmt.Errorf("cannot register handler with an empty version for %#v", *apiGroupInfo)
		}
	}

	for _, apiGroupInfo := range apiGroupInfos {
		if err := ols.installAPIResources(genericapiserver.APIGroupPrefix, apiGroupInfo); err != nil {
			return fmt.Errorf("unable to install api resources: %v", err)
		}

		if apiGroupInfo.PrioritizedVersions[0].String() == overlayapi.SchemeGroupVersion.String() {
			var found bool
			for _, ws := range ols.GenericAPIServer.Handler.GoRestfulContainer.RegisteredWebServices() {
				if ws.RootPath() == path.Join(genericapiserver.APIGroupPrefix, overlayapi.SchemeGroupVersion.String()) {
					ols.crdHandler.SetRootWebService(ws)
					found = true
				}
			}
			if !found {
				klog.WarningDepth(2, fmt.Sprintf("failed to find a root WebServices for %s", overlayapi.SchemeGroupVersion))
			}
		}

		// setup discovery
		// Install the version handler.
		// Add a handler at /apis/<groupName> to enumerate all versions supported by this group.
		apiVersionsForDiscovery := []metav1.GroupVersionForDiscovery{}
		for _, groupVersion := range apiGroupInfo.PrioritizedVersions {
			// Check the config to make sure that we elide versions that don't have any resources
			if len(apiGroupInfo.VersionedResourcesStorageMap[groupVersion.Version]) == 0 {
				continue
			}
			apiVersionsForDiscovery = append(apiVersionsForDiscovery, metav1.GroupVersionForDiscovery{
				GroupVersion: groupVersion.String(),
				Version:      groupVersion.Version,
			})
		}
		preferredVersionForDiscovery := metav1.GroupVersionForDiscovery{
			GroupVersion: apiGroupInfo.PrioritizedVersions[0].String(),
			Version:      apiGroupInfo.PrioritizedVersions[0].Version,
		}
		apiGroup := metav1.APIGroup{
			Name:             apiGroupInfo.PrioritizedVersions[0].Group,
			Versions:         apiVersionsForDiscovery,
			PreferredVersion: preferredVersionForDiscovery,
		}
		ols.GenericAPIServer.DiscoveryGroupManager.AddGroup(apiGroup)
		ols.GenericAPIServer.Handler.GoRestfulContainer.Add(genericdiscovery.NewAPIGroupHandler(ols.GenericAPIServer.Serializer, apiGroup).WebService())
	}
	return nil
}

// installAPIResources is a private method for installing the REST storage backing each api groupversionresource
// copied from k8s.io/apiserver/pkg/server/genericapiserver.go and modified
func (ols *OverlayAPIServer) installAPIResources(apiPrefix string, apiGroupInfo *genericapiserver.APIGroupInfo) error {
	var resourceInfos []*storageversion.ResourceInfo
	for _, groupVersion := range apiGroupInfo.PrioritizedVersions {
		if len(apiGroupInfo.VersionedResourcesStorageMap[groupVersion.Version]) == 0 {
			klog.Warningf("Skipping API %v because it has no resources.", groupVersion)
			continue
		}

		apiGroupVersion := ols.getAPIGroupVersion(apiGroupInfo, groupVersion, apiPrefix)
		if apiGroupInfo.OptionsExternalVersion != nil {
			apiGroupVersion.OptionsExternalVersion = apiGroupInfo.OptionsExternalVersion
		}

		apiGroupVersion.MaxRequestBodyBytes = ols.maxRequestBodyBytes

		r, err := apiGroupVersion.InstallREST(ols.GenericAPIServer.Handler.GoRestfulContainer)
		if err != nil {
			return fmt.Errorf("unable to setup API %v: %v", apiGroupInfo, err)
		}

		resourceInfos = append(resourceInfos, r...)
	}

	// API installation happens before we start listening on the handlers,
	// therefore it is safe to register ResourceInfos here. The handler will block
	// write requests until the storage versions of the targeting resources are updated.
	ols.GenericAPIServer.StorageVersionManager.AddResourceInfo(resourceInfos...)

	return nil
}

// a private method that copied from k8s.io/apiserver/pkg/server/genericapiserver.go and modified
func (ols *OverlayAPIServer) getAPIGroupVersion(apiGroupInfo *genericapiserver.APIGroupInfo, groupVersion schema.GroupVersion, apiPrefix string) *genericapi.APIGroupVersion {
	storage := make(map[string]rest.Storage)
	for k, v := range apiGroupInfo.VersionedResourcesStorageMap[groupVersion.Version] {
		storage[strings.ToLower(k)] = v
	}
	version := ols.newAPIGroupVersion(apiGroupInfo, groupVersion)
	version.Root = apiPrefix
	version.Storage = storage
	return version
}

// a private method that copied from k8s.io/apiserver/pkg/server/genericapiserver.go and modified
func (ols *OverlayAPIServer) newAPIGroupVersion(apiGroupInfo *genericapiserver.APIGroupInfo, groupVersion schema.GroupVersion) *genericapi.APIGroupVersion {
	return &genericapi.APIGroupVersion{
		GroupVersion:     groupVersion,
		MetaGroupVersion: apiGroupInfo.MetaGroupVersion,

		ParameterCodec:        apiGroupInfo.ParameterCodec,
		Serializer:            apiGroupInfo.NegotiatedSerializer,
		Creater:               apiGroupInfo.Scheme, //nolint:misspell
		Convertor:             apiGroupInfo.Scheme,
		ConvertabilityChecker: apiGroupInfo.Scheme,
		UnsafeConvertor:       runtime.UnsafeObjectConvertor(apiGroupInfo.Scheme),
		Defaulter:             apiGroupInfo.Scheme,
		Typer:                 apiGroupInfo.Scheme,
		Linker:                runtime.SelfLinker(meta.NewAccessor()),

		EquivalentResourceRegistry: ols.GenericAPIServer.EquivalentResourceRegistry,

		MinRequestTimeout:   time.Duration(ols.minRequestTimeout) * time.Second,
		Authorizer:          ols.GenericAPIServer.Authorizer,
		MaxRequestBodyBytes: ols.maxRequestBodyBytes,
	}
}
