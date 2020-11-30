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

	"github.com/Nerzal/gocloak/v7"
	tenantv1alpha1 "github.com/netgroup-polito/CrownLabs/operators/tenant-operator/api/v1alpha1"
	"github.com/netgroup-polito/CrownLabs/operators/tenant-operator/controllers"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	klog.Info("HELLO")
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	kcURL := takeEnvVar("KEYCLOAK_URL")
	kcAdminUser := takeEnvVar("KEYCLOAK_ADMIN_USER")
	kcAdminPsw := takeEnvVar("KEYCLOAK_ADMIN_PSW")

	// la go routine parte allo startup
	// prende il token e lo salva nel secret
	// ogni tot controlla il token e in caso lo refresha

	// informer sul secret che triggera la routine per cambiare il token dela struct dei reconciler
	// config := ctrl.GetConfigOrDie()
	// clientset, err := kubernetes.NewForConfigOrDie()

	kcClient := gocloak.NewClient(kcURL)

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
	klog.Info("HELLO")

	// k8sClient := kubernetes.NewForConfigOrDie(ctrl.GetConfigOrDie())
	// kcSecret, err := k8sClient.CoreV1().Secrets("").Get("keycloakSecret", metav1.GetOptions{})
	// if err != nil {
	// 	klog.Error("Error when getting keycloak secret")
	// 	klog.Fatal(err)
	// } else {
	// 	klog.Info(kcSecret)
	// }

	token, kcErr := kcClient.LoginAdmin(context.Background(), kcAdminUser, kcAdminPsw, "master")

	if kcErr != nil {
		setupLog.Error(kcErr, "unable to login as admin on keycloak")
		os.Exit(1)
	}

	if err = (&controllers.TenantReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Tenant"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Tenant")
		os.Exit(1)
	}
	if err = (&controllers.WorkspaceReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		KcClient: kcClient,
		KcToken:  token,
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

	var kcSecret v1.Secret
	kcSecretLookUpKey := types.NamespacedName{
		Name:      "keycloakSecret",
		Namespace: "",
	}
	k8sClient := mgr.GetClient()
	if err := k8sClient.Get(context.Background(), kcSecretLookUpKey, &kcSecret); err != nil {
		klog.Error("Error when getting keycloak secret")
		klog.Fatal(err)
	}
	klog.Info(kcSecret)

}

func takeEnvVar(s string) (ret string) {
	ret = os.Getenv(s)
	if ret == "" {
		setupLog.Info("missing required env " + s)
		os.Exit(1)
	}
	return ret
}
