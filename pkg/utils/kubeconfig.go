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

import (
	"errors"
	"fmt"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	componentbaseconfig "k8s.io/component-base/config"
)

// createBasicKubeConfig creates a basic, general KubeConfig object that then can be extended
func createBasicKubeConfig(serverURL, clusterName, userName string, caCert []byte) *clientcmdapi.Config {
	// Use the cluster and the username as the context name
	contextName := fmt.Sprintf("%s@%s", userName, clusterName)

	var insecureSkipTLSVerify bool
	if caCert == nil {
		insecureSkipTLSVerify = true
	}

	return &clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   serverURL,
				InsecureSkipTLSVerify:    insecureSkipTLSVerify,
				CertificateAuthorityData: caCert,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  clusterName,
				AuthInfo: userName,
			},
		},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{},
		CurrentContext: contextName,
	}
}

// CreateKubeConfigWithToken creates a KubeConfig object with access to the API server with a token
func CreateKubeConfigWithToken(serverURL, token string, caCert []byte) *clientcmdapi.Config {
	userName := "external-crd"
	clusterName := "external-crd-cluster"
	config := createBasicKubeConfig(serverURL, clusterName, userName, caCert)
	config.AuthInfos[userName] = &clientcmdapi.AuthInfo{
		Token: token,
	}
	return config
}

// LoadsKubeConfig tries to load kubeconfig from specified kubeconfig file or in-cluster config
func LoadsKubeConfig(clientConnectionCfg *componentbaseconfig.ClientConnectionConfiguration) (*rest.Config, error) {
	if clientConnectionCfg == nil {
		return nil, errors.New("nil ClientConnectionConfiguration")
	}

	var cfg *rest.Config
	var clientConfig *clientcmdapi.Config
	var err error

	switch clientConnectionCfg.Kubeconfig {
	case "":
		// use in-cluster config
		cfg, err = rest.InClusterConfig()
	default:
		clientConfig, err = clientcmd.LoadFromFile(clientConnectionCfg.Kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("error while loading kubeconfig from file %v: %v", clientConnectionCfg.Kubeconfig, err)
		}
		cfg, err = clientcmd.NewDefaultClientConfig(*clientConfig, &clientcmd.ConfigOverrides{}).ClientConfig()
	}

	if err != nil {
		return nil, err
	}

	// apply qps and burst settings
	//cfg.QPS = clientConnectionCfg.QPS
	//cfg.Burst = int(clientConnectionCfg.Burst)
	return cfg, nil
}

// GenerateKubeConfigFromToken composes a kubeconfig from token
func GenerateKubeConfigFromToken(serverURL, token string, caCert []byte, flowRate int) (*rest.Config, error) {
	clientConfig := CreateKubeConfigWithToken(serverURL, token, caCert)
	config, err := clientcmd.NewDefaultClientConfig(*clientConfig, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("error while creating kubeconfig: %v", err)
	}

	if flowRate < 0 {
		flowRate = 1
	}

	// here we magnify the default qps and burst in client-go
	//config.QPS = rest.DefaultQPS * float32(flowRate)
	//config.Burst = rest.DefaultBurst * flowRate

	return config, nil
}
