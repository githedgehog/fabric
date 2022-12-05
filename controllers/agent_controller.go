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
	"os"
	"os/exec"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	fabricv1alpha1 "github.com/githedgehog/fabric/api/v1alpha1"
)

// Definitions to manage status conditions
const (
	typeAgentAvailable = "Available"
	typeAgentReady     = "Ready"
)

// AgentReconciler reconciles a Agent object
type AgentReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Hostname  string
	Namespace string
}

//+kubebuilder:rbac:groups=fabric.githedgehog.com,resources=agents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=fabric.githedgehog.com,resources=agents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=fabric.githedgehog.com,resources=agents/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Agent object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling", "name", req.Name, "namespace", req.Namespace)

	if req.Name != r.Hostname || req.Namespace != r.Namespace {
		return ctrl.Result{}, nil
	}

	agent := &fabricv1alpha1.Agent{}
	err := r.Get(ctx, req.NamespacedName, agent)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("Agent resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get agent")
		return ctrl.Result{}, err
	}

	if agent.Status.Conditions == nil || len(agent.Status.Conditions) == 0 {
		// The following implementation will update the status
		meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
			Type:   typeAgentAvailable,
			Status: metav1.ConditionTrue, Reason: "Reconciling",
			Message: fmt.Sprintf("Agent (%s) is Available", agent.Name),
		})

		if err := r.Status().Update(ctx, agent); err != nil {
			log.Error(err, "Failed to update Agent status")
			return ctrl.Result{}, err
		}
	}

	for _, task := range agent.Spec.Tasks {
		if task.Vlan != nil {
			log.Info("Executing vlan task", "id", task.Vlan.Id, "untagged", task.Vlan.Untagged, "port", task.Vlan.Port)

			cmd := exec.Command(
				"/tmp/sonic-set-vlan.sh",
				task.Vlan.Port,
				strconv.Itoa(task.Vlan.Id),
				strconv.FormatBool(task.Vlan.Untagged),
			)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stdout

			// run command
			if err := cmd.Run(); err != nil {
				log.Error(err, "Error while executing vlan task")
			}
		}
	}

	meta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
		Type:   typeAgentReady,
		Status: metav1.ConditionTrue, Reason: "Ready",
		Message: fmt.Sprintf("Agent (%s) is Ready", agent.Name),
	})

	if err := r.Status().Update(ctx, agent); err != nil {
		log.Error(err, "Failed to update Agent status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// TODO: watch for specifically named agent
		For(&fabricv1alpha1.Agent{}).
		Complete(r)
}
