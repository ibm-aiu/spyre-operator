/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	certmanagerscheme "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/scheme"
	uberzap "go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// +kubebuilder:scaffold:imports
	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	"github.com/ibm-aiu/spyre-operator/controllers"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func initScheme() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(spyrev1alpha1.AddToScheme(scheme))
	utilruntime.Must(certmanagerscheme.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var renewDeadline time.Duration
	var webhookAdder int
	var webhookCertPath string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.DurationVar(&renewDeadline, "leader-lease-renew-deadline", 0,
		"Set the leader lease renew deadline duration (e.g. \"10s\") of the controller manager. "+
			"Only enabled when the --leader-elect flag is set. "+
			"If undefined, the renew deadline defaults to the controller-runtime manager's default RenewDeadline. "+
			"By setting this option, the LeaseDuration is also set as RenewDeadline + 5s.")
	flag.IntVar(&webhookAdder, "webhook-addr", 9443, "The address the webhook endpoint binds to.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "/tmp/k8s-webhook-server/serving-certs", "The path of server certification file") //nolint:lll

	initScheme()

	options := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "f0f631de.spyre.ibm.com",
	}

	if enableLeaderElection {
		if int(renewDeadline) != 0 {
			leaseDuration := renewDeadline + 5*time.Second
			options.RenewDeadline = &renewDeadline
			options.LeaseDuration = &leaseDuration
		} else {
			// refer to default settings in NFD operator to tolerate 60s of kube-apiserver disruption:
			// https://github.com/openshift/cluster-nfd-operator/blob/master/pkg/leaderelection/leaderelection.go
			leaseDuration := 137 * time.Second
			renewDeadline = 107 * time.Second
			retryDuration := 26 * time.Second
			options.RenewDeadline = &renewDeadline
			options.LeaseDuration = &leaseDuration
			options.RetryPeriod = &retryDuration
		}
	}

	rest.SetDefaultWarningHandler(
		rest.NewWarningWriter(os.Stderr, rest.WarningWriterOptions{
			// only print a given warning the first time we receive it
			Deduplicate: true,
			// don't print in color
			Color: false,
		},
		),
	)
	cfg := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(cfg, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to start manager: %s\n", err)
		os.Exit(1)
	}
	ctx := context.Background()
	reconciler := &controllers.SpyreClusterPolicyReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err = reconciler.SetupWithManager(ctx, cfg, mgr); err != nil {
		fmt.Fprintf(os.Stderr, "unable to create controller: %s\n", err)
		os.Exit(1)
	}

	nodeLabelerReconciler := &controllers.NodeLabelerReconciler{Client: mgr.GetClient()}
	if err := nodeLabelerReconciler.SetupWithManager(ctx, mgr); err != nil {
		fmt.Fprintf(os.Stderr, "unable to create node labeler controller: %s\n", err)
		os.Exit(1)
	}

	levelEnabler := uberzap.LevelEnablerFunc(func(level zapcore.Level) bool {
		return level >= reconciler.GetLogLevel()
	})
	opts := zap.Options{
		Development: true,
		Level:       levelEnabler,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	zlogger := zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(zlogger)

	// set zap for client-go logger
	klog.SetLogger(zlogger.WithName("client-go"))

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
