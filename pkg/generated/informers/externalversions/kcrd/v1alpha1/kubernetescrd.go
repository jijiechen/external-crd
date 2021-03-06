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
// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	kcrdv1alpha1 "github.com/jijiechen/external-crd/pkg/apis/kcrd/v1alpha1"
	versioned "github.com/jijiechen/external-crd/pkg/generated/clientset/versioned"
	internalinterfaces "github.com/jijiechen/external-crd/pkg/generated/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/jijiechen/external-crd/pkg/generated/listers/kcrd/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// KubernetesCrdInformer provides access to a shared informer and lister for
// KubernetesCrds.
type KubernetesCrdInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.KubernetesCrdLister
}

type kubernetesCrdInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewKubernetesCrdInformer constructs a new informer for KubernetesCrd type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewKubernetesCrdInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredKubernetesCrdInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredKubernetesCrdInformer constructs a new informer for KubernetesCrd type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredKubernetesCrdInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.KcrdV1alpha1().KubernetesCrds(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.KcrdV1alpha1().KubernetesCrds(namespace).Watch(context.TODO(), options)
			},
		},
		&kcrdv1alpha1.KubernetesCrd{},
		resyncPeriod,
		indexers,
	)
}

func (f *kubernetesCrdInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredKubernetesCrdInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *kubernetesCrdInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&kcrdv1alpha1.KubernetesCrd{}, f.defaultInformer)
}

func (f *kubernetesCrdInformer) Lister() v1alpha1.KubernetesCrdLister {
	return v1alpha1.NewKubernetesCrdLister(f.Informer().GetIndexer())
}
