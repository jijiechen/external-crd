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
	"context"
	"encoding/json"
	sys_errors "errors"
	"fmt"
	"github.com/jijiechen/external-crd/pkg/utils"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	clientgorest "k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	kcrd "github.com/jijiechen/external-crd/pkg/apis/kcrd/v1alpha1"
	kcrdclientset "github.com/jijiechen/external-crd/pkg/generated/clientset/versioned"
	applisters "github.com/jijiechen/external-crd/pkg/generated/listers/kcrd/v1alpha1"
)

const (
	CoreGroupPrefix  = "api"
	NamedGroupPrefix = "apis"

	// DefaultDeleteCollectionWorkers defines the default value for deleteCollectionWorkers
	DefaultDeleteCollectionWorkers = 2
)

// REST implements a RESTStorage for Shadow API
type REST struct {
	// name is the plural name of the resource.
	name string
	// shortNames is a list of suggested short names of the resource.
	shortNames []string
	// namespaced indicates if a resource is namespaced or not.
	namespaced bool
	// kind is the Kind for the resource (e.g. 'Foo' is the kind for a resource 'foo')
	kind string
	// group is the Group of the resource.
	group string
	// version is the Version of the resource.
	version string

	parameterCodec runtime.ParameterCodec

	dryRunClient clientgorest.Interface
	kcrdClient   *kcrdclientset.Clientset
	kcrdLister   applisters.KubernetesCrdLister

	// deleteCollectionWorkers is the maximum number of workers in a single
	// DeleteCollection call. Delete requests for the items in a collection
	// are issued in parallel.
	deleteCollectionWorkers int

	// namespace where Manifests are created
	reservedNamespace string
}

func getClusterNamespace(username string) (string, string, bool) {
	if len(username) == 0 {
		return "", "", false
	}

	prefix := fmt.Sprintf("system:serviceaccount:%s:biz-", utils.KcrdSystemNamespace)
	if !strings.HasPrefix(username, prefix) {
		return "", "", false
	}

	// name: system:serviceaccount:external-crd-system:biz-${random}-<cluster-id>-${random}-<namespaces>
	firstIdx := len(prefix)
	delimiter := username[firstIdx : firstIdx+utils.UsernameRandomDelimiterLength+1]

	lastIdx := strings.LastIndex(username, delimiter)
	// delimiter only found once
	if lastIdx == firstIdx {
		return "", "", false
	}

	clusterStart := firstIdx + len(delimiter)
	clusterEnd := lastIdx - 1

	return username[clusterStart:clusterEnd], username[clusterEnd+1+len(delimiter):], true
}

// Create inserts a new item into Manifest according to the unique key from the object.
func (r *REST) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	clusterID, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	// dry-run
	actualRes, err := r.dryRunCreate(ctx, obj, createValidation, options)
	if err != nil {
		return nil, err
	}

	// next we create manifest to store the actualRes
	kcrdRes := &kcrd.KubernetesCrd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getNormalizedManifestName(clusterID, actualRes.GetNamespace(), actualRes.GetName()),
			Namespace: r.reservedNamespace,
			Labels:    actualRes.GetLabels(), // reuse labels from original object, which is useful for label selector
		},
		Manifest: runtime.RawExtension{
			Object: actualRes,
		},
	}

	if kcrdRes.Labels == nil {
		kcrdRes.Labels = map[string]string{}
	}
	kcrdRes.Labels[utils.ConfigGroupLabel] = r.group
	kcrdRes.Labels[utils.ConfigVersionLabel] = r.version
	kcrdRes.Labels[utils.ConfigKindLabel] = r.kind
	kcrdRes.Labels[utils.ConfigNameLabel] = actualRes.GetName()
	kcrdRes.Labels[utils.ConfigClusterLabel] = clusterID
	kcrdRes, err = r.kcrdClient.KcrdV1alpha1().KubernetesCrds(kcrdRes.Namespace).Create(ctx, kcrdRes, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			return nil, errors.NewAlreadyExists(schema.GroupResource{Group: r.group, Resource: r.name}, actualRes.GetName())
		}
		return nil, err
	}
	return transformManifest(kcrdRes)
}

// Get retrieves the item from Manifest.
func (r *REST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	clusterID, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	var manifest *kcrd.KubernetesCrd
	if len(options.ResourceVersion) == 0 {
		manifest, err = r.kcrdLister.KubernetesCrds(r.reservedNamespace).Get(
			r.getNormalizedManifestName(clusterID, request.NamespaceValue(ctx), name))
	} else {
		manifest, err = r.kcrdClient.KcrdV1alpha1().KubernetesCrds(r.reservedNamespace).
			Get(ctx, r.getNormalizedManifestName(clusterID, request.NamespaceValue(ctx), name), *options)
	}
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NewNotFound(schema.GroupResource{Group: r.group, Resource: r.name}, name)
		}
		return nil, errors.NewInternalError(err)
	}
	return transformManifest(manifest)
}

// Update performs an atomic update and set of the object. Returns the result of the update
// or an error. If the registry allows create-on-update, the create flow will be executed.
// A bool is returned along with the object and any errors, to indicate object creation.
func (r *REST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo,
	createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc,
	forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// We are explicitly taking forceAllowCreate as false.
	// TODO: forceAllowCreate could be true
	resource, subresource := r.getResourceName()
	if len(subresource) > 0 {
		// all these overlay apis are considered as manifest templates, updating subresources, such as 'status' makes no sense.
		err := errors.NewMethodNotSupported(schema.GroupResource{Group: r.group, Resource: r.name}, "")
		err.ErrStatus.Message = fmt.Sprintf("%s are considered as manifests, which make no sense to update manifests' %s",
			resource, subresource)
		return nil, false, err
	}

	clusterID, err := getUser(ctx)
	if err != nil {
		return nil, false, err
	}
	manifest, err := r.kcrdLister.KubernetesCrds(r.reservedNamespace).Get(
		r.getNormalizedManifestName(clusterID, request.NamespaceValue(ctx), name))
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, false, errors.NewNotFound(schema.GroupResource{Group: r.group, Resource: r.name}, name)
		}
		return nil, false, errors.NewInternalError(err)
	}

	oldObj := &unstructured.Unstructured{}
	if err = json.Unmarshal(manifest.Manifest.Raw, oldObj); err != nil {
		return nil, false, errors.NewInternalError(err)
	}

	newObj, err := objInfo.UpdatedObject(ctx, oldObj)
	if err != nil {
		return nil, false, err
	}
	// Now we've got a fully formed object. Validators that apiserver handling chain wants to enforce can be called.
	if updateValidation != nil {
		if err := updateValidation(ctx, newObj.DeepCopyObject(), oldObj.DeepCopyObject()); err != nil {
			return nil, false, err
		}
	}
	result := newObj.(*unstructured.Unstructured)
	trimResult(result)

	// in case labels get changed
	manifestCopy := manifest.DeepCopy()
	if manifestCopy.Labels == nil {
		manifestCopy.Labels = map[string]string{}
	}
	for k, v := range result.GetLabels() {
		manifestCopy.Labels[k] = v
	}
	manifestCopy.Labels[utils.ConfigGroupLabel] = r.group
	manifestCopy.Labels[utils.ConfigVersionLabel] = r.version
	if r.kind != "Scale" {
		manifestCopy.Labels[utils.ConfigKindLabel] = r.kind
	}
	manifestCopy.Labels[utils.ConfigNameLabel] = result.GetName()
	manifestCopy.Labels[utils.ConfigNamespaceLabel] = result.GetNamespace()
	manifestCopy.Manifest.Reset()
	manifestCopy.Manifest.Object = result
	// save the updates
	manifestCopy, err = r.kcrdClient.KcrdV1alpha1().KubernetesCrds(r.reservedNamespace).Update(ctx, manifestCopy, *options)
	if err != nil {
		return nil, false, err
	}

	result, err = transformManifest(manifestCopy)
	return result, err != nil, err
}

// Delete removes the item from storage.
// options can be mutated by rest.BeforeDelete due to a graceful deletion strategy.
func (r *REST) Delete(ctx context.Context, name string, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	clusterID, err := getUser(ctx)
	if err != nil {
		return nil, false, err
	}

	err = r.kcrdClient.KcrdV1alpha1().KubernetesCrds(r.reservedNamespace).
		Delete(ctx, r.getNormalizedManifestName(clusterID, request.NamespaceValue(ctx), name), *options)
	if err != nil {
		if errors.IsNotFound(err) {
			err = errors.NewNotFound(schema.GroupResource{Group: r.group, Resource: r.name}, name)
		}
	}
	return nil, err == nil, err
}

// DeleteCollection removes all items returned by List with a given ListOptions from storage.
//
// DeleteCollection is currently NOT atomic. It can happen that only subset of objects
// will be deleted from storage, and then an error will be returned.
// In case of success, the list of deleted objects will be returned.
// Copied from k8s.io/apiserver/pkg/registry/generic/registry/store.go and modified.
func (r *REST) DeleteCollection(ctx context.Context, deleteValidation rest.ValidateObjectFunc, options *metav1.DeleteOptions, listOptions *internalversion.ListOptions) (runtime.Object, error) {
	if listOptions == nil {
		listOptions = &internalversion.ListOptions{}
	} else {
		listOptions = listOptions.DeepCopy()
	}

	listObj, err := r.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	items, err := meta.ExtractList(listObj)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		// Nothing to delete, return now
		return listObj, nil
	}
	// Spawn a number of goroutines, so that we can issue requests to storage
	// in parallel to speed up deletion.
	// It is proportional to the number of items to delete, up to
	// deleteCollectionWorkers (it doesn't make much sense to spawn 16
	// workers to delete 10 items).
	workersNumber := r.deleteCollectionWorkers
	if workersNumber > len(items) {
		workersNumber = len(items)
	}
	if workersNumber < 1 {
		workersNumber = 1
	}
	wg := sync.WaitGroup{}
	toProcess := make(chan int, 2*workersNumber)
	errs := make(chan error, workersNumber+1)

	go func() {
		defer utilruntime.HandleCrash(func(panicReason interface{}) {
			errs <- fmt.Errorf("DeleteCollection distributor panicked: %v", panicReason)
		})
		for i := 0; i < len(items); i++ {
			toProcess <- i
		}
		close(toProcess)
	}()

	wg.Add(workersNumber)
	for i := 0; i < workersNumber; i++ {
		go func() {
			// panics don't cross goroutine boundaries
			defer utilruntime.HandleCrash(func(panicReason interface{}) {
				errs <- fmt.Errorf("DeleteCollection goroutine panicked: %v", panicReason)
			})
			defer wg.Done()

			for index := range toProcess {
				accessor, err := meta.Accessor(items[index])
				if err != nil {
					errs <- err
					return
				}
				// DeepCopy the deletion options because individual graceful deleters communicate changes via a mutating
				// function in the delete strategy called in the delete method.  While that is always ugly, it works
				// when making a single call.  When making multiple calls via delete collection, the mutation applied to
				// pod/A can change the option ultimately used for pod/B.
				if _, _, err := r.Delete(ctx, accessor.GetName(), deleteValidation, options.DeepCopy()); err != nil && !errors.IsNotFound(err) {
					klog.V(4).InfoS("Delete object in DeleteCollection failed", "object", klog.KObj(accessor), "err", err)
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errs:
		return nil, err
	default:
		return listObj, nil
	}
}

// Watch makes a matcher for the given label and field.
func (r *REST) Watch(ctx context.Context, options *internalversion.ListOptions) (watch.Interface, error) {
	label, err := r.convertListOptionsToLabels(ctx, options)
	if err != nil {
		return nil, err
	}

	klog.V(5).Infof("%v", label)
	watcher, err := r.kcrdClient.KcrdV1alpha1().KubernetesCrds(r.reservedNamespace).Watch(ctx, metav1.ListOptions{
		LabelSelector:        label.String(),
		FieldSelector:        "", // explicitly set FieldSelector to an empty string
		Watch:                options.Watch,
		AllowWatchBookmarks:  options.AllowWatchBookmarks,
		ResourceVersion:      options.ResourceVersion,
		ResourceVersionMatch: options.ResourceVersionMatch,
		TimeoutSeconds:       options.TimeoutSeconds,
		Limit:                options.Limit,
		Continue:             options.Continue,
	})
	watchWrapper := utils.NewWatchWrapper(ctx, watcher, func(object runtime.Object) runtime.Object {
		// transform object here
		if _, ok := object.(*metav1.Status); ok {
			return object
		}

		if manifest, ok := object.(*kcrd.KubernetesCrd); ok {
			obj, err := transformManifest(manifest)
			if err != nil {
				klog.ErrorDepth(3, fmt.Sprintf("failed to transform Manifest %s: %v", klog.KObj(manifest), err))
				return manifest
			}
			return obj
		}

		return object
	}, utils.DefaultWatchSize)
	if err == nil {
		go watchWrapper.Run()
	}
	return watchWrapper, err
}

// List returns a list of items matching labels.
func (r *REST) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	label, err := r.convertListOptionsToLabels(ctx, options)
	if err != nil {
		return nil, err
	}

	manifests, err := r.kcrdClient.KcrdV1alpha1().KubernetesCrds(r.reservedNamespace).List(ctx, metav1.ListOptions{
		LabelSelector:        label.String(),
		FieldSelector:        "", // explicitly set FieldSelector to an empty string
		Watch:                options.Watch,
		AllowWatchBookmarks:  options.AllowWatchBookmarks,
		ResourceVersion:      options.ResourceVersion,
		ResourceVersionMatch: options.ResourceVersionMatch,
		TimeoutSeconds:       options.TimeoutSeconds,
		Limit:                options.Limit,
		Continue:             options.Continue,
	})
	if err != nil {
		return nil, err
	}

	result := &unstructured.UnstructuredList{}
	orignalGVK := r.GroupVersionKind(schema.GroupVersion{})
	result.SetAPIVersion(orignalGVK.GroupVersion().String())
	result.SetKind(r.getListKind())
	result.SetResourceVersion(manifests.ResourceVersion)
	result.SetContinue(manifests.Continue)
	// remainingItemCount will always be nil, since we're using non-empty label selectors.
	// This is a limitation on Kubernetes side.
	if len(manifests.Items) == 0 {
		return result, nil
	}
	for _, manifest := range manifests.Items {
		obj, err := transformManifest(&manifest)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *obj)
	}
	return result, nil
}

func (r *REST) NewList() runtime.Object {
	// Here the list GVK "meta.k8s.io/v1 List" is just a symbol,
	// since the real GVK will be set when List()
	newObj := &unstructured.UnstructuredList{}
	newObj.SetAPIVersion(metav1.SchemeGroupVersion.String())
	newObj.SetKind("List")
	return newObj
}

func (r *REST) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	tableConvertor := rest.NewDefaultTableConvertor(schema.GroupResource{Group: r.group, Resource: r.name})
	return tableConvertor.ConvertToTable(ctx, object, tableOptions)
}

func (r *REST) ShortNames() []string {
	return r.shortNames
}

func (r *REST) SetShortNames(ss []string) {
	r.shortNames = ss
}

func (r *REST) SetName(name string) {
	r.name = name
}

func (r *REST) NamespaceScoped() bool {
	return r.namespaced
}

func (r *REST) SetNamespaceScoped(namespaceScoped bool) {
	r.namespaced = namespaceScoped
}

func (r *REST) Categories() []string {
	return []string{utils.Category}
}

func (r *REST) SetGroup(group string) {
	r.group = group
}

func (r *REST) SetVersion(version string) {
	r.version = version
}

func (r *REST) SetKind(kind string) {
	r.kind = kind
}

func (r *REST) New() runtime.Object {
	newObj := &unstructured.Unstructured{}
	orignalGVK := r.GroupVersionKind(schema.GroupVersion{})
	newObj.SetGroupVersionKind(orignalGVK)
	return newObj
}

func (r *REST) GroupVersionKind(_ schema.GroupVersion) schema.GroupVersionKind {
	// use original GVK
	return r.GroupVersion().WithKind(r.kind)
}

func (r *REST) GroupVersion() schema.GroupVersion {
	return schema.GroupVersion{
		Group:   r.group,
		Version: r.version,
	}
}

func (r *REST) normalizeRequest(req *clientgorest.Request, namespace string) *clientgorest.Request {
	if len(r.group) == 0 {
		req.Prefix(CoreGroupPrefix, r.version)
	} else {
		req.Prefix(NamedGroupPrefix, r.group, r.version)
	}
	if r.namespaced {
		req.Namespace(namespace)
	}
	return req
}

func getUser(ctx context.Context) (string, error) {
	username, ok := request.UserFrom(ctx)
	if !ok {
		return "", errors.NewUnauthorized("No user info provided.")
	}

	// users are service accounts
	// name: external-crd-system:${random}-<cluster-id>-${random}-<namespaces>
	clusterID, authorizedNS, ok := getClusterNamespace(username.GetName())
	if !ok {
		return "", errors.NewForbidden(schema.GroupResource{}, "", sys_errors.New("invalid kcrd username format"))
	}

	actualNS := request.NamespaceValue(ctx)
	if len(actualNS) == 0 || actualNS != authorizedNS {
		return "", errors.NewForbidden(schema.GroupResource{}, "",
			sys_errors.New(fmt.Sprintf("can not operate resource in '%s'. allowed namespace: '%s'",
				actualNS, authorizedNS)))
	}

	return clusterID, nil
}

// getNormalizedManifestName will converge generateLegacyNameForManifest and generateNameForManifest
func (r *REST) getNormalizedManifestName(clusterid, namespace, name string) string {
	resource, _ := r.getResourceName()
	// resource is a word ("[a-z]([-a-z0-9]*[a-z0-9])?") without "."
	// namespace is a word ("[a-z]([-a-z0-9]*[a-z0-9])?") without "."
	// so we use "." for concatenation
	return fmt.Sprintf("%s.%s.%s.%s", resource, clusterid, namespace, name)
}

func (r *REST) dryRunCreate(ctx context.Context, obj runtime.Object, _ rest.ValidateObjectFunc, options *metav1.CreateOptions) (*unstructured.Unstructured, error) {
	objNamespace := request.NamespaceValue(ctx)

	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, errors.NewBadRequest(fmt.Sprintf("not a Unstructured object: %T", obj))
	}

	// check whether the given namespace name is valid
	if r.namespaced || r.kind == "Namespace" {
		fieldPath := field.NewPath("metadata", "namespace")
		if r.kind == "Namespace" {
			fieldPath = field.NewPath("metadata", "name")
		}
		if errs := apimachineryvalidation.ValidateNamespaceName(objNamespace, false); len(errs) > 0 {
			allErrs := field.ErrorList{field.Invalid(fieldPath, objNamespace, strings.Join(errs, ","))}
			return nil, errors.NewInvalid(r.GroupVersionKind(schema.GroupVersion{}).GroupKind(), u.GetName(), allErrs)
		}
	}

	labels := u.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[utils.ObjectCreatedByLabel] = utils.ExternalCrdAppName
	u.SetLabels(labels)

	if r.kind != "Namespace" && r.namespaced {
		u.SetNamespace(r.reservedNamespace)
	}
	// use reserved namespace (default to be "clusternet-reserved") to avoid error "namespaces not found"
	dryRunNamespace := r.reservedNamespace
	if r.kind == "Namespace" {
		dryRunNamespace = ""
	}

	body, err := u.MarshalJSON()
	if err != nil {
		return nil, errors.NewBadRequest(fmt.Sprintf("failed to marshal to json: %v", u.Object))
	}

	result := &unstructured.Unstructured{}
	klog.V(7).Infof("creating %s with %s", r.kind, body)
	resource, _ := r.getResourceName()
	// first we dry-run the creation
	req := r.dryRunClient.Post().
		Resource(resource).
		Param("dryRun", "All").
		VersionedParams(options, r.parameterCodec).
		Body(body)
	err = r.normalizeRequest(req, dryRunNamespace).Do(ctx).Into(result)
	if err != nil {
		return nil, err
	}

	if r.kind != "Namespace" && r.namespaced {
		// set original namespace back
		result.SetNamespace(objNamespace)
	}

	// trim metadata
	trimResult(result)
	return result, nil
}

func (r *REST) convertListOptionsToLabels(ctx context.Context, options *internalversion.ListOptions) (labels.Selector, error) {
	clusterID, err := getUser(ctx)
	if err != nil {
		return nil, err
	}

	label := labels.Everything()
	if options != nil && options.LabelSelector != nil {
		label = options.LabelSelector
	}
	if options != nil && options.FieldSelector != nil {
		rqmts := options.FieldSelector.Requirements()
		for _, rqmt := range rqmts {
			var selectorKey string
			switch rqmt.Field {
			case "metadata.name":
				selectorKey = utils.ConfigNameLabel
			default:
				return nil, errors.NewInternalError(fmt.Errorf("unable to recognize selector key %s", rqmt.Field))
			}
			requirement, err := labels.NewRequirement(selectorKey, rqmt.Operator, []string{rqmt.Value})
			if err != nil {
				return nil, err
			}
			label = label.Add(*requirement)
		}
	}

	// apply default kind label
	kindRequirement, err := labels.NewRequirement(utils.ConfigKindLabel, selection.Equals, []string{r.kind})
	if err != nil {
		return nil, err
	}
	label = label.Add(*kindRequirement)

	// apply default namespace label
	namespace := request.NamespaceValue(ctx)
	nsRequirement, err := labels.NewRequirement(utils.ConfigNamespaceLabel, selection.Equals, []string{namespace})
	if err != nil {
		return nil, err
	}
	label = label.Add(*nsRequirement)

	clsRequirement, err := labels.NewRequirement(utils.ConfigClusterLabel, selection.Equals, []string{clusterID})
	if err != nil {
		return nil, err
	}
	label = label.Add(*clsRequirement)
	return label, nil
}

func (r *REST) getResourceName() (string, string) {
	// is subresource
	if strings.Contains(r.name, "/") {
		resources := strings.Split(r.name, "/")
		return resources[0], resources[1]
	}

	return r.name, ""
}

func transformManifest(crdResource *kcrd.KubernetesCrd) (*unstructured.Unstructured, error) {
	result := &unstructured.Unstructured{}
	if err := json.Unmarshal(crdResource.Manifest.Raw, result); err != nil {
		return nil, errors.NewInternalError(err)
	}
	result.SetGeneration(crdResource.Generation)
	result.SetCreationTimestamp(crdResource.CreationTimestamp)
	result.SetResourceVersion(crdResource.ResourceVersion)
	result.SetUID(crdResource.UID)
	result.SetDeletionGracePeriodSeconds(crdResource.DeletionGracePeriodSeconds)
	result.SetDeletionTimestamp(crdResource.DeletionTimestamp)
	result.SetFinalizers(crdResource.Finalizers)

	annotations := result.GetAnnotations()
	result.SetAnnotations(annotations)

	return result, nil
}

func trimResult(result *unstructured.Unstructured) {
	// trim common metadata
	// metadata.uid cannot be trimmed, which will be used for checking when patching.
	unstructured.RemoveNestedField(result.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(result.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(result.Object, "metadata", "resourceVersion")
}

func (r *REST) getListKind() string {
	if strings.Contains(r.name, "/") {
		return r.kind
	}
	return fmt.Sprintf("%sList", r.kind)
}

// NewREST returns a RESTStorage object that will work against API services.
func NewREST(dryRunClient clientgorest.Interface, clusternetclient *kcrdclientset.Clientset, parameterCodec runtime.ParameterCodec,
	manifestLister applisters.KubernetesCrdLister, reservedNamespace string) *REST {
	return &REST{
		dryRunClient:            dryRunClient,
		kcrdClient:              clusternetclient,
		kcrdLister:              manifestLister,
		parameterCodec:          parameterCodec,
		deleteCollectionWorkers: DefaultDeleteCollectionWorkers, // currently we only set a default value for deleteCollectionWorkers
		reservedNamespace:       reservedNamespace,
	}
}

var _ rest.GroupVersionKindProvider = &REST{}
var _ rest.CategoriesProvider = &REST{}
var _ rest.ShortNamesProvider = &REST{}
var _ rest.StandardStorage = &REST{}
