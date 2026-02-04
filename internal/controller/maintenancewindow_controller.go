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
	"math"
	"strconv"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

	// Add finalizer before creating external resource to prevent orphaning
	if !controllerutil.ContainsFinalizer(mw, myFinalizerName) {
		controllerutil.AddFinalizer(mw, myFinalizerName)
		if err := r.Update(ctx, mw); err != nil {
			return ctrl.Result{}, err
		}
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

	// Convert duration from Go format to minutes (round up to avoid shortening)
	durationMinutes := int(math.Ceil(mw.Spec.Duration.Minutes()))
	if durationMinutes < 1 {
		durationMinutes = 1
	}

	if !mw.Status.Ready {
		// Create new maintenance window
		createReq := uptimerobot.CreateMaintenanceWindowRequest{
			Name:            mw.Spec.Name,
			AutoAddMonitors: mw.Spec.AutoAddMonitors,
			Interval:        mw.Spec.Interval,
			Time:            mw.Spec.StartTime,
			Duration:        durationMinutes,
			Days:            mw.Spec.Days,
			MonitorIDs:      monitorIDs,
		}
		// Only include date for "once" interval
		if mw.Spec.Interval == "once" {
			createReq.Date = mw.Spec.StartDate
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
			MonitorIDs:      &monitorIDs,
		}

		result, err := urclient.UpdateMaintenanceWindow(ctx, mw.Status.ID, updateReq)
		if err != nil {
			// If maintenance window was deleted out-of-band, recreate it
			if uptimerobot.IsNotFound(err) {
				logger.Info("Maintenance window not found in UptimeRobot, recreating", "id", mw.Status.ID)
				createReq := uptimerobot.CreateMaintenanceWindowRequest{
					Name:            mw.Spec.Name,
					AutoAddMonitors: mw.Spec.AutoAddMonitors,
					Interval:        mw.Spec.Interval,
					Time:            mw.Spec.StartTime,
					Duration:        durationMinutes,
					Days:            mw.Spec.Days,
					MonitorIDs:      monitorIDs,
				}
				// Only include date for "once" interval
				if mw.Spec.Interval == "once" {
					createReq.Date = mw.Spec.StartDate
				}

				result, err := urclient.CreateMaintenanceWindow(ctx, createReq)
				if err != nil {
					return ctrl.Result{}, fmt.Errorf("failed to recreate maintenance window: %w", err)
				}

				mw.Status.ID = strconv.Itoa(result.ID)
				mw.Status.MonitorCount = len(result.MonitorIDs)
				if err := r.Status().Update(ctx, mw); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: mw.Spec.SyncInterval.Duration}, nil
			}
			return ctrl.Result{}, fmt.Errorf("failed to update maintenance window: %w", err)
		}

		mw.Status.MonitorCount = len(result.MonitorIDs)
		if err := r.Status().Update(ctx, mw); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: mw.Spec.SyncInterval.Duration}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MaintenanceWindowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Index MaintenanceWindows by referenced Monitor names for efficient lookups
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &uptimerobotv1.MaintenanceWindow{}, "spec.monitorRefs", func(obj client.Object) []string {
		mw := obj.(*uptimerobotv1.MaintenanceWindow)
		var names []string
		for _, ref := range mw.Spec.MonitorRefs {
			if ref.Name != "" {
				names = append(names, ref.Name)
			}
		}
		return names
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&uptimerobotv1.MaintenanceWindow{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(
			&uptimerobotv1.Monitor{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				monitor := obj.(*uptimerobotv1.Monitor)

				// Find all MaintenanceWindows that reference this Monitor
				var mwList uptimerobotv1.MaintenanceWindowList
				if err := r.List(ctx, &mwList,
					client.InNamespace(monitor.Namespace),
					client.MatchingFields{"spec.monitorRefs": monitor.Name},
				); err != nil {
					return nil
				}

				// Create reconcile requests for each MaintenanceWindow
				requests := make([]reconcile.Request, len(mwList.Items))
				for i, mw := range mwList.Items {
					requests[i] = reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: mw.Namespace,
							Name:      mw.Name,
						},
					}
				}
				return requests
			}),
		).
		Named("maintenancewindow").
		Complete(r)
}
