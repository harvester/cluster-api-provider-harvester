/*
Copyright 2025 SUSE.

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

// Package main is the entry point for the controller manager.
package main

import (
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrastructurev1alpha1 "github.com/harvester/cluster-api-provider-harvester/api/v1alpha1"
	infrastructurev1beta1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
	"github.com/harvester/cluster-api-provider-harvester/internal/controller"
)

const (
	webhookPort = 9443
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	utilruntime.Must(infrastructurev1alpha1.AddToScheme(scheme))
	utilruntime.Must(infrastructurev1beta1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string

	var metricsOptions metrics.Options

	var enableLeaderElection bool

	var enableWebhooks bool

	var probeAddr string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":9440", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&enableWebhooks, "enable-webhooks", false,
		"Enable validating webhooks for HarvesterMachine and HarvesterCluster resources.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	metricsOptions.BindAddress = metricsAddr
	mgrOpts := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsOptions,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "1e1658d6.cluster.x-k8s.io",
	}

	if enableWebhooks {
		webhookServer := webhook.NewServer(webhook.Options{
			Port: webhookPort,
		})
		mgrOpts.WebhookServer = webhookServer
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup the context to be used for the controller and manager
	ctx := ctrl.SetupSignalHandler()

	err = (&controller.HarvesterMachineReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(ctx, mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HarvesterMachine")
		os.Exit(1)
	}

	err = (&controller.HarvesterClusterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(ctx, mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HarvesterCluster")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if enableWebhooks {
		err = infrastructurev1beta1.SetupHarvesterMachineWebhookWithManager(mgr)
		if err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "HarvesterMachine v1beta1")
			os.Exit(1)
		}

		err = infrastructurev1alpha1.SetupHarvesterMachineWebhookWithManager(mgr)
		if err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "HarvesterMachine")
			os.Exit(1)
		}

		err = infrastructurev1beta1.SetupHarvesterClusterWebhookWithManager(mgr)
		if err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "HarvesterCluster v1beta1")
			os.Exit(1)
		}

		err = infrastructurev1alpha1.SetupHarvesterClusterWebhookWithManager(mgr)
		if err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "HarvesterCluster")
			os.Exit(1)
		}

		setupLog.Info("webhooks enabled")
	}

	err = mgr.AddHealthzCheck("healthz", healthz.Ping)
	if err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	err = mgr.AddReadyzCheck("readyz", healthz.Ping)
	if err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")

	err = mgr.Start(ctx)
	if err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
