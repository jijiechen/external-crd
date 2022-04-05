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

package main

import (
	goflag "flag"
	"fmt"
	"github.com/jijiechen/external-crd/pkg/apiserver"
	"github.com/spf13/cobra"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/klog/v2"

	"context"
	"github.com/jijiechen/external-crd/pkg/utils"
)

var (
	// the command name
	cmdName = "external-crd"
)

func main() {
	klog.InitFlags(nil)
	defer klog.Flush()
	rand.Seed(time.Now().UTC().UnixNano())

	ctx := utils.GracefulStopWithContext()
	command := NewExternalCrdServerCmd(ctx)
	pflag.CommandLine.SetNormalizeFunc(utils.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// NewExternalCrdServerCmd creates a *cobra.Command object with default parameters
func NewExternalCrdServerCmd(ctx context.Context) *cobra.Command {
	opts, err := apiserver.NewOverlayServerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	cmd := &cobra.Command{
		Use:  cmdName,
		Long: `Running in external cluster, responsible for storing CRDs`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := utils.PrintAndExitIfRequested(cmdName); err != nil {
				klog.Exit(err)
			}

			if err := opts.Complete(); err != nil {
				klog.Exit(err)
			}
			if err := opts.Validate(); err != nil {
				klog.Exit(err)
			}

			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				klog.V(1).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
			})

			server, err := apiserver.NewOverlayServer(opts)
			if err != nil {
				klog.Exit(err)
			}
			if err := server.Run(ctx); err != nil {
				klog.Exit(err)
			}
		},
	}

	// bind flags
	flags := cmd.Flags()
	utils.AddVersionFlag(flags)
	opts.AddFlags(flags)
	utilfeature.DefaultMutableFeatureGate.AddFlag(flags)

	return cmd
}
