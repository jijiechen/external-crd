/*
Copyright 2022 Jijie Chen.

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
// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/jijiechen/external-crd/pkg/apis/kcrd/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// KubernetesCrdLister helps list KubernetesCrds.
// All objects returned here must be treated as read-only.
type KubernetesCrdLister interface {
	// List lists all KubernetesCrds in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.KubernetesCrd, err error)
	// KubernetesCrds returns an object that can list and get KubernetesCrds.
	KubernetesCrds(namespace string) KubernetesCrdNamespaceLister
	KubernetesCrdListerExpansion
}

// kubernetesCrdLister implements the KubernetesCrdLister interface.
type kubernetesCrdLister struct {
	indexer cache.Indexer
}

// NewKubernetesCrdLister returns a new KubernetesCrdLister.
func NewKubernetesCrdLister(indexer cache.Indexer) KubernetesCrdLister {
	return &kubernetesCrdLister{indexer: indexer}
}

// List lists all KubernetesCrds in the indexer.
func (s *kubernetesCrdLister) List(selector labels.Selector) (ret []*v1alpha1.KubernetesCrd, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.KubernetesCrd))
	})
	return ret, err
}

// KubernetesCrds returns an object that can list and get KubernetesCrds.
func (s *kubernetesCrdLister) KubernetesCrds(namespace string) KubernetesCrdNamespaceLister {
	return kubernetesCrdNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// KubernetesCrdNamespaceLister helps list and get KubernetesCrds.
// All objects returned here must be treated as read-only.
type KubernetesCrdNamespaceLister interface {
	// List lists all KubernetesCrds in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.KubernetesCrd, err error)
	// Get retrieves the KubernetesCrd from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.KubernetesCrd, error)
	KubernetesCrdNamespaceListerExpansion
}

// kubernetesCrdNamespaceLister implements the KubernetesCrdNamespaceLister
// interface.
type kubernetesCrdNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all KubernetesCrds in the indexer for a given namespace.
func (s kubernetesCrdNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.KubernetesCrd, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.KubernetesCrd))
	})
	return ret, err
}

// Get retrieves the KubernetesCrd from the indexer for a given namespace and name.
func (s kubernetesCrdNamespaceLister) Get(name string) (*v1alpha1.KubernetesCrd, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("kubernetescrd"), name)
	}
	return obj.(*v1alpha1.KubernetesCrd), nil
}