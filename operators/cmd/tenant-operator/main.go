/*


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
	"context"
	"flag"
	"os"
	"time"

	"github.com/Nerzal/gocloak/v7"
	tenantv1alpha1 "github.com/netgroup-polito/CrownLabs/operators/api/v1alpha1"
	"github.com/netgroup-polito/CrownLabs/operators/pkg/controllers"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = tenantv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "f547a6ba.crownlabs.polito.it",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	kcURL := takeEnvVar("KEYCLOAK_URL")
	kcAdminUser := takeEnvVar("KEYCLOAK_ADMIN_USER")
	kcAdminPsw := takeEnvVar("KEYCLOAK_ADMIN_PSW")
	targetKcRealm := takeEnvVar("KEYCLOAK_TARGET_REALM")
	kcClient := gocloak.NewClient(kcURL)
	token, kcErr := kcClient.LoginAdmin(context.Background(), kcAdminUser, kcAdminPsw, "master")
	if kcErr != nil {
		setupLog.Error(kcErr, "unable to login as admin on keycloak")
		os.Exit(1)
	}

	kcRenewTokenTicker := time.NewTicker(5 * time.Second)
	// don't know if the following line is needed, probably not
	defer kcRenewTokenTicker.Stop()
	go func() {
		for {
			<-kcRenewTokenTicker.C
			CheckAndRenewToken(context.Background(), kcClient, token, kcAdminUser, kcAdminPsw, 5*time.Minute)
		}
	}()

	if err = (&controllers.TenantReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Tenant")
		os.Exit(1)
	}
	if err = (&controllers.WorkspaceReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		KcClient:      kcClient,
		KcToken:       token,
		TargetKcRealm: targetKcRealm,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Workspace")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

}

func takeEnvVar(s string) (ret string) {
	ret = os.Getenv(s)
	if ret == "" {
		klog.Infof("missing required env %s", s)
		os.Exit(1)
	}
	return ret
}

// CheckAndRenewToken checks if the token is expired, if so it renews it
func CheckAndRenewToken(ctx context.Context, kcClient gocloak.GoCloak, token *gocloak.JWT, kcAdminUser string, kcAdminPsw string, expireLimit time.Duration) error {
	_, claims, err := kcClient.DecodeAccessToken(ctx, token.AccessToken, "master", "")
	if err != nil {
		klog.Error(err, "problems when decoding token")
		return err
	}
	tokenExpiresIn := time.Unix(int64((*claims)["exp"].(float64)), 0).Sub(time.Now())

	if tokenExpiresIn < expireLimit {
		newToken, err := kcClient.LoginAdmin(ctx, kcAdminUser, kcAdminPsw, "master")
		if err != nil {
			klog.Error(err, "Error when renewing token")
			return err
		}
		*token = *newToken
		klog.Info("Keycloak token renewed")
		return nil
	}
	return nil
}
