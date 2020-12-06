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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Nerzal/gocloak/v7"
	crownlabsv1alpha1 "github.com/netgroup-polito/CrownLabs/operators/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	KcClient      gocloak.GoCloak
	KcToken       *gocloak.JWT
	TargetKcRealm string
}

// +kubebuilder:rbac:groups=crownlabs.polito.it,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=crownlabs.polito.it,resources=workspaces/status,verbs=get;update;patch

// Reconcile reconciles the state of a workspace resource
func (r *WorkspaceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()

	var ws crownlabsv1alpha1.Workspace
	accToken := r.KcToken.AccessToken
	targetRealm := r.TargetKcRealm
	targetClientID, err := GetClientID(ctx, r.KcClient, accToken, targetRealm, "k8s")
	if err != nil {
		klog.Error(err, "Error when getting client id for k8s")
		return ctrl.Result{}, err
	}

	if err := r.Get(ctx, req.NamespacedName, &ws); err != nil {
		// reconcile was triggered by a delete request
		klog.Infof("Workspace %s deleted", req.Name)
		if err := deleteWorkspaceRoles(ctx, r.KcClient, accToken, targetRealm, targetClientID, req.Name); err != nil {
			klog.Errorf("Error when deleting roles of workspace %s", req.NamespacedName)
			klog.Error(err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	klog.Infof("Reconciling workspace %s", req.Name)

	nsName := fmt.Sprintf("workspace-%s", ws.Name)
	ns := v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}

	nsOpRes, err := ctrl.CreateOrUpdate(ctx, r.Client, &ns, func() error {
		updateNamespace(ws, &ns, nsName)
		return ctrl.SetControllerReference(&ws, &ns, r.Scheme)
	})
	if err != nil {
		klog.Errorf("Unable to create or update namespace of workspace %s", ws.Name)
		klog.Error(err)
		// update status of workspace with failed namespace creation
		ws.Status.Namespace.Created = false
		ws.Status.Namespace.Name = ""
		// return anyway the error to allow new reconcile, independently of outcome of status update
		if err := r.Status().Update(ctx, &ws); err != nil {
			klog.Error(err, "Unable to update status after namespace creation failed")
		}
		return ctrl.Result{}, err
	}
	klog.Infof("Namespace %s for workspace %s %s", nsName, req.Name, nsOpRes)

	// update status of workspace with info about namespace, success
	ws.Status.Namespace.Created = true
	ws.Status.Namespace.Name = nsName

	if err := createKcRolesForWorkspace(ctx, r.KcClient, accToken, targetRealm, targetClientID, ws.Name); err != nil {
		if ws.Status.Subscriptions == nil {
			ws.Status.Subscriptions = make(map[string]crownlabsv1alpha1.SubscriptionStatus)
		}
		ws.Status.Subscriptions["keycloak"] = crownlabsv1alpha1.SubscrFailed
		if err := r.Status().Update(ctx, &ws); err != nil {
			// if status update fails, still try to reconcile later
			klog.Error(err, "Unable to update status with failed keycloak")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}
	if ws.Status.Subscriptions == nil {
		ws.Status.Subscriptions = make(map[string]crownlabsv1alpha1.SubscriptionStatus)
	}
	ws.Status.Subscriptions["keycloak"] = crownlabsv1alpha1.SubscrOk

	// everything should went ok, update status before exiting reconcile
	if err := r.Status().Update(ctx, &ws); err != nil {
		// if status update fails, still try to reconcile later
		klog.Error(err, "Unable to update status before exiting reconciler")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crownlabsv1alpha1.Workspace{}).
		Complete(r)
}

func updateNamespace(ws crownlabsv1alpha1.Workspace, ns *v1.Namespace, wsnsName string) {
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	ns.Labels["crownlabs.polito.it/type"] = "workspace"
}

func createKcRolesForWorkspace(ctx context.Context, kcClient gocloak.GoCloak, token string, realmName string, targetClientID string, wsName string) error {

	newUserRoleName := fmt.Sprintf("workspace-%s:user", wsName)
	if err := createKcRole(ctx, kcClient, token, realmName, targetClientID, newUserRoleName); err != nil {
		klog.Error("Could not create user role")
		return err
	}

	newAdminRoleName := fmt.Sprintf("workspace-%s:admin", wsName)
	if err := createKcRole(ctx, kcClient, token, realmName, targetClientID, newAdminRoleName); err != nil {
		klog.Error("Could not create admin role")
		return err
	}
	return nil
}

func deleteWorkspaceRoles(ctx context.Context, kcClient gocloak.GoCloak, token string, realmName string, targetClientID string, wsName string) error {
	userRoleName := fmt.Sprintf("workspace-%s:user", wsName)
	if err := kcClient.DeleteClientRole(ctx, token, realmName, targetClientID, userRoleName); err != nil {
		klog.Error("Could not delete user role")
		return err
	}

	adminRoleName := fmt.Sprintf("workspace-%s:admin", wsName)
	if err := kcClient.DeleteClientRole(ctx, token, realmName, targetClientID, adminRoleName); err != nil {
		klog.Error("Could not delete admin role")
		return err
	}
	return nil
}
