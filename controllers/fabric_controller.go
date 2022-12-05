/*
Copyright 2022 The Hedgehog Authors.

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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	fabricv1alpha1 "github.com/githedgehog/fabric/api/v1alpha1"
)

// Definitions to manage status conditions
const (
	typeFabricAvailable = "Available"
)

// FabricReconciler reconciles a Fabric object
type FabricReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=fabric.githedgehog.com,resources=fabrics,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=fabric.githedgehog.com,resources=fabrics/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=fabric.githedgehog.com,resources=fabrics/finalizers,verbs=update
//+kubebuilder:rbac:groups=fabric.githedgehog.com,resources=devices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=fabric.githedgehog.com,resources=devices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=fabric.githedgehog.com,resources=devices/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Fabric object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *FabricReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	log := log.FromContext(ctx)
	log.Info("Reconciling", "name", req.Name, "namespace", req.Namespace)

	// Fetch the Fabric instance
	// The purpose is check if the Custom Resource for the Kind Fabric
	// is applied on the cluster if not we return nil to stop the reconciliation
	fabric := &fabricv1alpha1.Fabric{}
	err := r.Get(ctx, req.NamespacedName, fabric)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("fabric resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get fabric")
		return ctrl.Result{}, err
	}

	if fabric.Status.Conditions == nil || len(fabric.Status.Conditions) == 0 {
		// The following implementation will update the status
		meta.SetStatusCondition(&fabric.Status.Conditions, metav1.Condition{
			Type:   typeFabricAvailable,
			Status: metav1.ConditionTrue, Reason: "Reconciling",
			Message: fmt.Sprintf("Fabric (%s) is Available", fabric.Name),
		})

		if err := r.Status().Update(ctx, fabric); err != nil {
			log.Error(err, "Failed to update Fabric status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FabricReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fabricv1alpha1.Fabric{}).
		// TODO: consider switching to owned by corresponding controllers converting labels into owned ref
		Watches(&source.Kind{Type: &fabricv1alpha1.Device{}}, handler.EnqueueRequestsFromMapFunc(enqueueIfOwned)).
		Watches(&source.Kind{Type: &fabricv1alpha1.Link{}}, handler.EnqueueRequestsFromMapFunc(enqueueIfOwned)).
		Complete(r)
}

func enqueueIfOwned(obj client.Object) []reconcile.Request {
	labels := obj.GetLabels()

	// TODO: make a const in API for the label or move to spec / owned
	fabricName := "default"
	if val, ok := labels["fabric.githedgehog.com/name"]; ok {
		fabricName = val
	}
	fabricNamespace := obj.GetNamespace()
	if val, ok := labels["fabric.githedgehog.com/namespace"]; ok {
		fabricNamespace = val
	}

	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: fabricNamespace, Name: fabricName}}}
}
