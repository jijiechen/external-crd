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
	overlayapiserver "github.com/jijiechen/external-crd/pkg/apiserver/overlay"
	"github.com/jijiechen/external-crd/pkg/utils"
	crdclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	crdinformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/version"
	componentbaseconfig "k8s.io/component-base/config"
	componentbaseoptions "k8s.io/component-base/config/options"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/controller-manager/pkg/clientbuilder"
	aggregatorinformers "k8s.io/kube-aggregator/pkg/client/informers/externalversions"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/pflag"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/plugin/namespace/lifecycle"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericfilters "k8s.io/apiserver/pkg/server/filters"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	kcrd "github.com/jijiechen/external-crd/pkg/generated/clientset/versioned"
	informers "github.com/jijiechen/external-crd/pkg/generated/informers/externalversions"
)

// OverlayServerOptions contains state for master/api server
type OverlayServerOptions struct {
	// No tunnel logging by default
	TunnelLogging bool

	// Whether the anonymous access is allowed by the kube-apiserver,
	// i.e. flag "--anonymous-auth=true" is set to kube-apiserver.
	// If enabled, then the deployers in Clusternet will use anonymous when proxying requests to child clusters.
	// If not, serviceaccount "clusternet-hub-proxy" will be used instead.
	AnonymousAuthSupported bool

	// default namespace to create Manifest in
	// default to be "clusternet-reserved"
	ReservedNamespace string

	RecommendedOptions *genericoptions.RecommendedOptions

	LoopbackSharedInformerFactory informers.SharedInformerFactory

	*ControllerOptions
}

// NewOverlayServerOptions returns a new OverlayServerOptions
func NewOverlayServerOptions() (*OverlayServerOptions, error) {
	controllerOpts, err := NewControllerOptions("external-crd", utils.KcrdSystemNamespace)
	if err != nil {
		return nil, err
	}
	//controllerOpts.ClientConnection.QPS = rest.DefaultQPS * float32(10)
	//controllerOpts.ClientConnection.Burst = int32(rest.DefaultBurst * 10)

	return &OverlayServerOptions{
		RecommendedOptions:     genericoptions.NewRecommendedOptions("fake", nil),
		AnonymousAuthSupported: true,
		ReservedNamespace:      utils.KcrdReservedNamespace,
		ControllerOptions:      controllerOpts,
	}, nil
}

// Validate validates OverlayServerOptions
func (o *OverlayServerOptions) Validate() error {
	errors := []error{}
	errors = append(errors, o.validateRecommendedOptions()...)
	return utilerrors.NewAggregate(errors)
}

// Complete fills in fields required to have valid data
func (o *OverlayServerOptions) Complete() error {
	o.RecommendedOptions.CoreAPI.CoreAPIKubeconfigPath = o.ClientConnection.Kubeconfig
	return nil
}

// Config returns config for the api server given OverlayServerOptions
func (o *OverlayServerOptions) Config() (*Config, error) {
	// TODO have a "real" external address
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	o.RecommendedOptions.ExtraAdmissionInitializers = func(c *genericapiserver.RecommendedConfig) ([]admission.PluginInitializer, error) {
		client, err := kcrd.NewForConfig(c.LoopbackClientConfig)
		if err != nil {
			return nil, err
		}
		informerFactory := informers.NewSharedInformerFactory(client, c.LoopbackClientConfig.Timeout)
		o.LoopbackSharedInformerFactory = informerFactory
		// TODO: add initializer
		return []admission.PluginInitializer{}, nil
	}

	// remove NamespaceLifecycle admission plugin explicitly
	o.RecommendedOptions.Admission.DisablePlugins = append(o.RecommendedOptions.Admission.DisablePlugins, lifecycle.PluginName)

	serverConfig := genericapiserver.NewRecommendedConfig(overlayapiserver.Codecs)
	serverConfig.Config.RequestTimeout = time.Duration(40) * time.Second // override default 60s
	serverConfig.LongRunningFunc = func(r *http.Request, requestInfo *apirequest.RequestInfo) bool {
		if values := r.URL.Query()["watch"]; len(values) > 0 {
			switch strings.ToLower(values[0]) {
			case "true":
				return true
			default:
				return false
			}
		}
		return genericfilters.BasicLongRunningRequestCheck(sets.NewString("watch"), sets.NewString())(r, requestInfo)
	}
	if err := o.recommendedOptionsApplyTo(serverConfig); err != nil {
		return nil, err
	}

	config := &Config{
		GenericConfig: serverConfig,
		ExtraConfig:   ExtraConfig{},
	}
	return config, nil
}

func (o *OverlayServerOptions) AddFlags(fs *pflag.FlagSet) {
	o.addRecommendedOptionsFlags(fs)
	o.ControllerOptions.AddFlags(fs)

	fs.BoolVar(&o.TunnelLogging, "enable-tunnel-logging", o.TunnelLogging, "Enable tunnel logging")
	fs.BoolVar(&o.AnonymousAuthSupported, "anonymous-auth-supported", o.AnonymousAuthSupported, "Whether the anonymous access is allowed by the 'core' kubernetes server")
	fs.StringVar(&o.ReservedNamespace, "reserved-namespace", o.ReservedNamespace, "The default namespace to create Manifest in")
}

func (o *OverlayServerOptions) addRecommendedOptionsFlags(fs *pflag.FlagSet) {
	// Copied from k8s.io/apiserver/pkg/server/options/recommended.go
	// and remove unused flags

	o.RecommendedOptions.SecureServing.AddFlags(fs)
	o.RecommendedOptions.Authentication.AddFlags(fs)
	o.RecommendedOptions.Authorization.AddFlags(fs)
	o.RecommendedOptions.Audit.LogOptions.AddFlags(fs)
	o.RecommendedOptions.Features.AddFlags(fs)
	// flag "kubeconfig" has been declared in o.ControllerOptions
	//o.RecommendedOptions.CoreAPI.AddFlags(fs) // --kubeconfig flag
}

func (o *OverlayServerOptions) validateRecommendedOptions() []error {
	// Copied from k8s.io/apiserver/pkg/server/options/recommended.go
	// and remove unused Validate

	errors := []error{}
	errors = append(errors, o.RecommendedOptions.SecureServing.Validate()...)
	errors = append(errors, o.RecommendedOptions.Authentication.Validate()...)
	errors = append(errors, o.RecommendedOptions.Authorization.Validate()...)
	errors = append(errors, o.RecommendedOptions.Audit.LogOptions.Validate()...)
	errors = append(errors, o.RecommendedOptions.Features.Validate()...)
	errors = append(errors, o.RecommendedOptions.CoreAPI.Validate()...)
	return errors
}

func (o *OverlayServerOptions) recommendedOptionsApplyTo(config *genericapiserver.RecommendedConfig) error {
	// Copied from k8s.io/apiserver/pkg/server/options/recommended.go
	// and remove unused ApplyTo

	if err := o.RecommendedOptions.SecureServing.ApplyTo(&config.Config.SecureServing, &config.Config.LoopbackClientConfig); err != nil {
		return err
	}
	if err := o.RecommendedOptions.Authentication.ApplyTo(&config.Config.Authentication, config.SecureServing, config.OpenAPIConfig); err != nil {
		return err
	}
	if err := o.RecommendedOptions.Authorization.ApplyTo(&config.Config.Authorization); err != nil {
		return err
	}
	if err := o.RecommendedOptions.Audit.ApplyTo(&config.Config); err != nil {
		return err
	}
	if err := o.RecommendedOptions.Features.ApplyTo(&config.Config); err != nil {
		return err
	}
	if err := o.RecommendedOptions.CoreAPI.ApplyTo(config); err != nil {
		return err
	}
	if initializers, err := o.RecommendedOptions.ExtraAdmissionInitializers(config); err != nil {
		return err
	} else if err := o.RecommendedOptions.Admission.ApplyTo(&config.Config, config.SharedInformerFactory, config.ClientConfig, o.RecommendedOptions.FeatureGate, initializers...); err != nil {
		return err
	}
	return nil
}

// ExtraConfig holds custom apiserver config
type ExtraConfig struct {
	// Place you custom config here.
}

// Config defines the config for the apiserver
type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

// ExternalCrdAPIServer contains state for a master/api server.
type ExternalCrdAPIServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

// CompletedConfig embeds a private pointer that cannot be instantiated outside of this package.
type CompletedConfig struct {
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (cfg *Config) Complete() CompletedConfig {
	c := completedConfig{
		cfg.GenericConfig.Complete(),
		&cfg.ExtraConfig,
	}

	c.GenericConfig.Version = &version.Info{
		Major: "1",
		Minor: "0",
	}

	return CompletedConfig{&c}
}

// New returns a new instance of ExternalCrdAPIServer from the given config.
func (c completedConfig) New(kubeclient *kubernetes.Clientset, kcrdclient *kcrd.Clientset,
	kcrdInformerFactory informers.SharedInformerFactory,
	aggregatorInformerFactory aggregatorinformers.SharedInformerFactory,
	clientBuilder clientbuilder.ControllerClientBuilder,
	reservedNamespace string) (*ExternalCrdAPIServer, error) {
	genericServer, err := c.GenericConfig.New("kcrd-server", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	s := &ExternalCrdAPIServer{
		GenericAPIServer: genericServer,
	}

	kcrdInformerFactory.Kcrd().V1alpha1().KubernetesCrds().Informer()
	aggregatorInformerFactory.Apiregistration().V1().APIServices().Informer()

	s.GenericAPIServer.AddPostStartHookOrDie("start-external-crd-overlay-apis", func(context genericapiserver.PostStartHookContext) error {
		if s.GenericAPIServer != nil {
			klog.Infof("install overlay apis...")
			crdInformerFactory := crdinformers.NewSharedInformerFactory(
				crdclientset.NewForConfigOrDie(clientBuilder.ConfigOrDie("crd-shared-informers")),
				5*time.Minute,
			)
			ss := overlayapiserver.NewOverlayAPIServer(s.GenericAPIServer, c.GenericConfig.MaxRequestBodyBytes,
				c.GenericConfig.MinRequestTimeout, c.GenericConfig.AdmissionControl, kubeclient.RESTClient(),
				kcrdclient,
				kcrdInformerFactory.Kcrd().V1alpha1().KubernetesCrds().Lister(),
				aggregatorInformerFactory.Apiregistration().V1().APIServices().Lister(),
				crdInformerFactory,
				reservedNamespace)

			crdInformerFactory.Start(context.StopCh)
			return ss.InstallOverlayAPIGroups(context.StopCh, kubeclient.DiscoveryClient)
		}

		select {
		case <-context.StopCh:
		}

		return nil
	})

	return s, nil
}

// ControllerOptions has all the params needed to run a Controller
type ControllerOptions struct {
	// LeaderElection defines the configuration of leader election client.
	LeaderElection componentbaseconfig.LeaderElectionConfiguration

	// ClientConnection specifies the kubeconfig file and client connection
	// settings for the proxy server to use when communicating with the apiserver.
	ClientConnection componentbaseconfig.ClientConnectionConfiguration
}

// NewControllerOptions returns a new ControllerOptions
func NewControllerOptions(resourceName, resourceNamespace string) (*ControllerOptions, error) {
	versionedClientConnection := componentbaseconfigv1alpha1.ClientConnectionConfiguration{}
	versionedLeaderElection := componentbaseconfigv1alpha1.LeaderElectionConfiguration{
		ResourceLock:      "lease", // Use lease-based leader election to reduce cost.
		ResourceName:      resourceName,
		ResourceNamespace: resourceNamespace,
	}
	// Use the default ClientConnectionConfiguration and LeaderElectionConfiguration options
	componentbaseconfigv1alpha1.RecommendedDefaultClientConnectionConfiguration(&versionedClientConnection)
	componentbaseconfigv1alpha1.RecommendedDefaultLeaderElectionConfiguration(&versionedLeaderElection)

	o := &ControllerOptions{
		ClientConnection: componentbaseconfig.ClientConnectionConfiguration{},
		LeaderElection:   componentbaseconfig.LeaderElectionConfiguration{},
	}

	controllerScheme := runtime.NewScheme()
	utilruntime.Must(componentbaseconfigv1alpha1.AddToScheme(controllerScheme))
	if err := controllerScheme.Convert(&versionedClientConnection, &o.ClientConnection, nil); err != nil {
		return nil, err
	}
	if err := controllerScheme.Convert(&versionedLeaderElection, &o.LeaderElection, nil); err != nil {
		return nil, err
	}

	return o, nil
}

// Validate validates ControllerOptions
func (o *ControllerOptions) Validate() error {
	errors := []error{}
	return utilerrors.NewAggregate(errors)
}

// Complete fills in fields required to have valid data
func (o *ControllerOptions) Complete() error {
	// TODO

	return nil
}

// AddFlags adds flags for ControllerOptions.
func (o *ControllerOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.ClientConnection.Kubeconfig, "kubeconfig", o.ClientConnection.Kubeconfig, "Path to a kubeconfig file pointing at the 'core' kubernetes server. Only required if out-of-cluster.")
	//fs.Float32Var(&o.ClientConnection.QPS, "kube-api-qps", o.ClientConnection.QPS, "QPS to use while talking with the 'core' kubernetes apiserver.")
	//fs.Int32Var(&o.ClientConnection.Burst, "kube-api-burst", o.ClientConnection.Burst, "Burst to use while talking with 'core' kubernetes apiserver.")

	componentbaseoptions.BindLeaderElectionFlags(&o.LeaderElection, fs)
	if err := fs.MarkHidden("leader-elect-resource-lock"); err != nil {
		klog.Errorf("failed to set a flag to hidden: %v", err)
	}
}
