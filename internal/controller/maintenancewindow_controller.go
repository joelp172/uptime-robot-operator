/*
Copyright 2025.

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

package controller

import (
	"context"
	"fmt"
	"strconv"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

// MaintenanceWindowReconciler reconciles a MaintenanceWindow object
type MaintenanceWindowReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=uptimerobot.com,resources=maintenancewindows,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=uptimerobot.com,resources=maintenancewindows/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=uptimerobot.com,resources=maintenancewindows/finalizers,verbs=update
//+kubebuilder:rbac:groups=uptimerobot.com,resources=monitors,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MaintenanceWindowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	mw := &uptimerobotv1.MaintenanceWindow{}
	if err := r.Get(ctx, req.NamespacedName, mw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	account := &uptimerobotv1.Account{}
	if err := GetAccount(ctx, r.Client, account, mw.Spec.Account.Name); err != nil {
		return ctrl.Result{}, err
	}

	apiKey, err := GetApiKey(ctx, r.Client, account)
	if err != nil {
		return ctrl.Result{}, err
	}
	urclient := uptimerobot.NewClient(apiKey)

	const myFinalizerName = "uptimerobot.com/finalizer"
	if !mw.DeletionTimestamp.IsZero() {
		// Object is being deleted
		if controllerutil.ContainsFinalizer(mw, myFinalizerName) {
			if mw.Spec.Prune && mw.Status.Ready {
				if err := urclient.DeleteMaintenanceWindow(ctx, mw.Status.ID); err != nil {
					return ctrl.Result{}, err
				}
			}

			controllerutil.RemoveFinalizer(mw, myFinalizerName)
			if err := r.Update(ctx, mw); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	// Resolve monitor IDs from monitorRefs
	monitorIDs := []int{}
	for _, ref := range mw.Spec.MonitorRefs {
		monitor := &uptimerobotv1.Monitor{}
		namespace := req.Namespace
		if ref.Name == "" {
			continue
		}

		if err := r.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      ref.Name,
		}, monitor); err != nil {
			logger.Info("Monitor not found, skipping", "monitor", ref.Name, "error", err)
			continue
		}

		if !monitor.Status.Ready || monitor.Status.ID == "" {
			logger.Info("Monitor not ready yet, skipping", "monitor", ref.Name)
			continue
		}

		id, err := strconv.Atoi(monitor.Status.ID)
		if err != nil {
			logger.Info("Invalid monitor ID, skipping", "monitor", ref.Name, "id", monitor.Status.ID)
			continue
		}

		monitorIDs = append(monitorIDs, id)
	}

	// Convert duration from Go format to minutes
	durationMinutes := int(mw.Spec.Duration.Duration.Minutes())
	if durationMinutes < 1 {
		durationMinutes = 1
	}

	if !mw.Status.Ready {
		// Create new maintenance window
		createReq := uptimerobot.CreateMaintenanceWindowRequest{
			Name:            mw.Spec.Name,
			AutoAddMonitors: mw.Spec.AutoAddMonitors,
			Interval:        mw.Spec.Interval,
			Date:            mw.Spec.StartDate,
			Time:            mw.Spec.StartTime,
			Duration:        durationMinutes,
			Days:            mw.Spec.Days,
			MonitorIDs:      monitorIDs,
		}

		result, err := urclient.CreateMaintenanceWindow(ctx, createReq)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create maintenance window: %w", err)
		}

		mw.Status.Ready = true
		mw.Status.ID = strconv.Itoa(result.ID)
		mw.Status.MonitorCount = len(result.MonitorIDs)
		if err := r.Status().Update(ctx, mw); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// Update existing maintenance window
		updateReq := uptimerobot.UpdateMaintenanceWindowRequest{
			Name:            mw.Spec.Name,
			AutoAddMonitors: &mw.Spec.AutoAddMonitors,
			Interval:        mw.Spec.Interval,
			Date:            mw.Spec.StartDate,
			Time:            mw.Spec.StartTime,
			Duration:        durationMinutes,
			Days:            mw.Spec.Days,
			MonitorIDs:      monitorIDs,
		}

		result, err := urclient.UpdateMaintenanceWindow(ctx, mw.Status.ID, updateReq)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update maintenance window: %w", err)
		}

		mw.Status.MonitorCount = len(result.MonitorIDs)
		if err := r.Status().Update(ctx, mw); err != nil {
			return ctrl.Result{}, err
		}
	}

	if !controllerutil.ContainsFinalizer(mw, myFinalizerName) {
		controllerutil.AddFinalizer(mw, myFinalizerName)
		if err := r.Update(ctx, mw); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: mw.Spec.SyncInterval.Duration}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MaintenanceWindowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&uptimerobotv1.MaintenanceWindow{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("maintenancewindow").
		Complete(r)
}
