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
	// +kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()
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

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "f547a6ba.crownlabs.polito.it",
	})
	if err != nil {
		klog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	kcURL := TakeEnvVar("KEYCLOAK_URL")
	kcAdminUser := TakeEnvVar("KEYCLOAK_ADMIN_USER")
	kcAdminPsw := TakeEnvVar("KEYCLOAK_ADMIN_PSW")
	kcClient, kcToken := prepareKcClient(kcURL, kcAdminUser, kcAdminPsw)

	go checkAndRenewTokenPeriodically(context.Background(), kcClient, kcToken, kcAdminUser, kcAdminPsw, 2*time.Minute, 5*time.Minute)

	targetKcRealm := TakeEnvVar("KEYCLOAK_TARGET_REALM")

	if err = (&controllers.TenantReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		klog.Error(err, "Unable to create controller for Tenant")
		os.Exit(1)
	}
	if err = (&controllers.WorkspaceReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		KcClient:      kcClient,
		KcToken:       kcToken,
		TargetKcRealm: targetKcRealm,
	}).SetupWithManager(mgr); err != nil {
		klog.Error(err, "unable to create controller for Workspace")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	klog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		klog.Error(err, "Problem running manager")
		os.Exit(1)
	}

}

// TakeEnvVar wrapper around og.GetEnv to better handle error in case env var is not specified
func TakeEnvVar(s string) (ret string) {
	ret = os.Getenv(s)
	if ret == "" {
		klog.Infof("Missing required env %s", s)
		os.Exit(1)
	}
	return ret
}

// checkAndRenewTokenPeriodically checks every intervalCheck if the token is about in less than expireLimit or is already expired, if so it renews it
func checkAndRenewTokenPeriodically(ctx context.Context, kcClient gocloak.GoCloak, token *gocloak.JWT, kcAdminUser string, kcAdminPsw string, intervalCheck time.Duration, expireLimit time.Duration) {

	kcRenewTokenTicker := time.NewTicker(intervalCheck)
	for {
		// wait intervalCheck
		<-kcRenewTokenTicker.C
		// take expiration date of token from tokenJWT claims
		_, claims, err := kcClient.DecodeAccessToken(ctx, token.AccessToken, "master", "")
		if err != nil {
			klog.Error(err, "Error when decoding token")
			os.Exit(1)
		}
		// convert expiration time in usable time
		tokenExpiresIn := time.Unix(int64((*claims)["exp"].(float64)), 0).Sub(time.Now())

		// if token is about to expire, renew it
		if tokenExpiresIn < expireLimit {
			newToken, err := kcClient.LoginAdmin(ctx, kcAdminUser, kcAdminPsw, "master")
			if err != nil {
				klog.Error(err, "Error when renewing token")
				os.Exit(1)
			}
			*token = *newToken
			klog.Info("Keycloak token renewed")
		}
	}
}

// prepareKcClient sets up a keycloak client with the specififed parameters and performs the first login
// function is exported to be used in tests
func prepareKcClient(kcURL, kcAdminUser, kcAdminPsw string) (gocloak.GoCloak, *gocloak.JWT) {

	kcClient := gocloak.NewClient(kcURL)
	token, kcErr := kcClient.LoginAdmin(context.Background(), kcAdminUser, kcAdminPsw, "master")
	if kcErr != nil {
		klog.Error(kcErr, "Unable to login as admin on keycloak")
		os.Exit(1)
	}
	return kcClient, token
}
