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

package apiserver

import (
	"context"
	"github.com/jijiechen/external-crd/pkg/utils"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/controller-manager/pkg/clientbuilder"
	"k8s.io/klog/v2"
	aggregatorclient "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	aggregatorinformers "k8s.io/kube-aggregator/pkg/client/informers/externalversions"

	kcrdapi "github.com/jijiechen/external-crd/pkg/apis/kcrd/v1alpha1"
	kcrd "github.com/jijiechen/external-crd/pkg/generated/clientset/versioned"
	informers "github.com/jijiechen/external-crd/pkg/generated/informers/externalversions"
)

// OverlayServer defines configuration for kcrd-hub
type OverlayServer struct {
	options *OverlayServerOptions

	kcrdInformerFactory       informers.SharedInformerFactory
	kubeInformerFactory       kubeinformers.SharedInformerFactory
	aggregatorInformerFactory aggregatorinformers.SharedInformerFactory

	kubeClient    *kubernetes.Clientset
	kcrdClient    *kcrd.Clientset
	clientBuilder clientbuilder.ControllerClientBuilder
}

// NewOverlayServer returns a new OverlayServer.
func NewOverlayServer(opts *OverlayServerOptions) (*OverlayServer, error) {
	config, err := utils.LoadsKubeConfig(&opts.ClientConnection)
	if err != nil {
		return nil, err
	}

	// creating the clientset
	rootClientBuilder := clientbuilder.SimpleControllerClientBuilder{
		ClientConfig: config,
	}
	kubeClient := kubernetes.NewForConfigOrDie(rootClientBuilder.ConfigOrDie("kcrd-kube-client"))
	kcrdClient := kcrd.NewForConfigOrDie(rootClientBuilder.ConfigOrDie("kcrd-server-client"))

	utilruntime.Must(kcrdapi.AddToScheme(scheme.Scheme))

	// creates the informer factory
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, utils.DefaultResync)
	kcrdInformerFactory := informers.NewSharedInformerFactory(kcrdClient, utils.DefaultResync)
	aggregatorInformerFactory := aggregatorinformers.NewSharedInformerFactory(aggregatorclient.
		NewForConfigOrDie(rootClientBuilder.ConfigOrDie("kcrd-server-kube-client")), utils.DefaultResync)

	server := &OverlayServer{
		options:                   opts,
		kubeClient:                kubeClient,
		kcrdClient:                kcrdClient,
		clientBuilder:             rootClientBuilder,
		kcrdInformerFactory:       kcrdInformerFactory,
		kubeInformerFactory:       kubeInformerFactory,
		aggregatorInformerFactory: aggregatorInformerFactory,
	}
	return server, nil
}

// Run starts a new OverlayAPIServer given OverlayServerOptions
func (s *OverlayServer) Run(ctx context.Context) error {
	klog.Info("starting external crd api server ...")
	config, err := s.options.Config()
	if err != nil {
		return err
	}

	server, err := config.Complete().New(
		s.kubeClient,
		s.kcrdClient,
		s.kcrdInformerFactory,
		s.aggregatorInformerFactory,
		s.clientBuilder,
		s.options.ReservedNamespace)
	if err != nil {
		return err
	}

	server.GenericAPIServer.AddPostStartHookOrDie("start-shared-informers-controllers",
		func(context genericapiserver.PostStartHookContext) error {
			klog.Infof("starting external-crd informers ...")
			// Start the informer factories to begin populating the informer caches
			// Start method is non-blocking and runs all registered informers in a dedicated goroutine.
			s.kubeInformerFactory.Start(context.StopCh)
			s.kcrdInformerFactory.Start(context.StopCh)
			s.aggregatorInformerFactory.Start(context.StopCh)
			config.GenericConfig.SharedInformerFactory.Start(context.StopCh)

			// waits for all started informers' cache got synced
			s.kubeInformerFactory.WaitForCacheSync(context.StopCh)
			s.kcrdInformerFactory.WaitForCacheSync(context.StopCh)
			s.aggregatorInformerFactory.WaitForCacheSync(context.StopCh)
			// TODO: uncomment this when module "k8s.io/apiserver" gets bumped to a higher version.
			// 		supports k8s.io/apiserver version skew (kcrd/kcrd#137)
			// config.GenericConfig.SharedInformerFactory.WaitForCacheSync(context.StopCh)

			select {
			case <-context.StopCh:
			}

			return nil
		},
	)

	return server.GenericAPIServer.PrepareRun().Run(ctx.Done())
}
