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

package controllers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nerzal/gocloak/v7"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	crownlabsv1alpha1 "github.com/netgroup-polito/CrownLabs/operators/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var kcClient gocloak.GoCloak
var kcToken *gocloak.JWT
var targetKcRealm = "testing"
var testEnv *envtest.Environment

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})

}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "deploy", "crds")},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = crownlabsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	kcURL := takeEnvVar("KEYCLOAK_URL")
	kcAdminUser := takeEnvVar("KEYCLOAK_ADMIN_USER")
	kcAdminPsw := takeEnvVar("KEYCLOAK_ADMIN_PSW")

	kcClient, kcToken = prepareKcClient(kcURL, kcAdminUser, kcAdminPsw)
	Expect(kcClient).ToNot(BeNil())
	Expect(kcToken).ToNot(BeNil())
	err = (&WorkspaceReconciler{
		Client:        k8sManager.GetClient(),
		Scheme:        k8sManager.GetScheme(),
		KcClient:      kcClient,
		KcToken:       kcToken,
		TargetKcRealm: targetKcRealm,
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred())
	}()

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).ToNot(BeNil())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

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

// takeEnvVar wrapper around og.GetEnv to better handle error in case env var is not specified
func takeEnvVar(s string) (ret string) {
	ret = os.Getenv(s)
	if ret == "" {
		klog.Infof("Missing required env %s", s)
		os.Exit(1)
	}
	return ret
}
