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
	"k8s.io/client-go/tools/record"
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

const (
	// intervalOnce represents the "once" interval type for maintenance windows
	intervalOnce = "once"
)

// MaintenanceWindowReconciler reconciles a MaintenanceWindow object
type MaintenanceWindowReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=uptimerobot.com,resources=maintenancewindows,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=uptimerobot.com,resources=maintenancewindows/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=uptimerobot.com,resources=maintenancewindows/finalizers,verbs=update
//+kubebuilder:rbac:groups=uptimerobot.com,resources=monitors,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MaintenanceWindowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	mw := &uptimerobotv1.MaintenanceWindow{}
	if err := r.Get(ctx, req.NamespacedName, mw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Update observedGeneration
	mw.Status.ObservedGeneration = mw.Generation

	account := &uptimerobotv1.Account{}
	if err := GetAccount(ctx, r.Client, account, mw.Spec.Account.Name); err != nil {
		if mw.Status.ID == "" {
			mw.Status.Ready = false
		}
		SetReadyCondition(&mw.Status.Conditions, false, ReasonReconcileError, fmt.Sprintf("Failed to get account: %v", err), mw.Generation)
		SetErrorCondition(&mw.Status.Conditions, true, ReasonReconcileError, fmt.Sprintf("Failed to get account: %v", err), mw.Generation)
		if updateErr := r.Status().Update(ctx, mw); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	apiKey, err := GetApiKey(ctx, r.Client, account)
	if err != nil {
		if mw.Status.ID == "" {
			mw.Status.Ready = false
		}
		SetReadyCondition(&mw.Status.Conditions, false, ReasonSecretNotFound, fmt.Sprintf("Failed to get API key: %v", err), mw.Generation)
		SetErrorCondition(&mw.Status.Conditions, true, ReasonSecretNotFound, fmt.Sprintf("Failed to get API key: %v", err), mw.Generation)
		if updateErr := r.Status().Update(ctx, mw); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}
	urclient := uptimerobot.NewClient(apiKey)

	const myFinalizerName = "uptimerobot.com/finalizer"
	if !mw.DeletionTimestamp.IsZero() {
		// Object is being deleted
		if controllerutil.ContainsFinalizer(mw, myFinalizerName) {
			// Initialize cleanup tracking on first deletion attempt
			if err := InitializeCleanupTracking(ctx, r.Client, mw); err != nil {
				return ctrl.Result{}, err
			}

			// Reload the maintenance window to get the updated annotations
			if err := r.Get(ctx, req.NamespacedName, mw); err != nil {
				return ctrl.Result{}, client.IgnoreNotFound(err)
			}

			// Define the cleanup function
			cleanupFunc := func(ctx context.Context) error {
				if !mw.Spec.Prune || !mw.Status.Ready {
					// Skip cleanup if Prune is false or resource is not ready
					return nil
				}
				return urclient.DeleteMaintenanceWindow(ctx, mw.Status.ID)
			}

			// Handle cleanup with retry and timeout logic
			result, _ := HandleFinalizerCleanup(ctx, CleanupOptions{
				Object:             mw,
				Conditions:         &mw.Status.Conditions,
				ObservedGeneration: mw.Generation,
				Recorder:           r.Recorder,
				CleanupFunc:        cleanupFunc,
			})

			// Update status with cleanup progress
			if updateErr := r.Status().Update(ctx, mw); updateErr != nil {
				return ctrl.Result{}, updateErr
			}

			// If cleanup failed and we shouldn't force-remove, requeue
			if !result.Success && !result.ForceRemove {
				return ctrl.Result{RequeueAfter: result.RequeueAfter}, nil
			}

			// Remove finalizer (either success or force-remove)
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
		logger.Info("Creating maintenance window",
			"name", mw.Spec.Name,
			"autoAddMonitors", mw.Spec.AutoAddMonitors,
			"interval", mw.Spec.Interval,
			"monitorIDs", monitorIDs,
		)
		// When autoAddMonitors is true, do NOT send monitorIds.
		// The API automatically adds all monitors; sending an empty list
		// would override that behaviour.
		var createMonitorIDs []int
		if !mw.Spec.AutoAddMonitors {
			createMonitorIDs = monitorIDs
		}

		createReq := uptimerobot.CreateMaintenanceWindowRequest{
			Name:            mw.Spec.Name,
			AutoAddMonitors: mw.Spec.AutoAddMonitors,
			Interval:        mw.Spec.Interval,
			Time:            mw.Spec.StartTime,
			Duration:        durationMinutes,
			Days:            mw.Spec.Days,
			MonitorIDs:      createMonitorIDs,
		}
		// Only include date for "once" interval
		if mw.Spec.Interval == intervalOnce {
			createReq.Date = mw.Spec.StartDate
		}

		result, err := urclient.CreateMaintenanceWindow(ctx, createReq)
		if err != nil {
			mw.Status.Ready = false
			msg := fmt.Sprintf("Failed to create maintenance window: %v", err)
			SetReadyCondition(&mw.Status.Conditions, false, ReasonAPIError, msg, mw.Generation)
			SetSyncedCondition(&mw.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to sync with UptimeRobot: %v", err), mw.Generation)
			SetErrorCondition(&mw.Status.Conditions, true, ReasonAPIError, msg, mw.Generation)
			if updateErr := r.Status().Update(ctx, mw); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, fmt.Errorf("failed to create maintenance window: %w", err)
		}

		mw.Status.Ready = true
		mw.Status.ID = strconv.Itoa(result.ID)
		mw.Status.MonitorCount = len(result.MonitorIDs)
		SetReadyCondition(&mw.Status.Conditions, true, ReasonReconcileSuccess, "MaintenanceWindow reconciled successfully", mw.Generation)
		SetSyncedCondition(&mw.Status.Conditions, true, ReasonSyncSuccess, "Successfully synced with UptimeRobot", mw.Generation)
		SetErrorCondition(&mw.Status.Conditions, false, ReasonReconcileSuccess, "", mw.Generation)
		if err := r.Status().Update(ctx, mw); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// Update existing maintenance window
		logger.Info("Updating maintenance window",
			"name", mw.Spec.Name,
			"autoAddMonitors", mw.Spec.AutoAddMonitors,
			"interval", mw.Spec.Interval,
			"monitorIDs", monitorIDs,
		)

		// When autoAddMonitors is true, do NOT send monitorIds in the update.
		// Sending an empty monitorIds list would override the auto-add behaviour
		// and disassociate all monitors.
		var monitorIDsPtr *[]int
		if !mw.Spec.AutoAddMonitors {
			monitorIDsPtr = &monitorIDs
		}

		updateReq := uptimerobot.UpdateMaintenanceWindowRequest{
			Name:            mw.Spec.Name,
			AutoAddMonitors: &mw.Spec.AutoAddMonitors,
			Interval:        mw.Spec.Interval,
			Time:            mw.Spec.StartTime,
			Duration:        durationMinutes,
			Days:            mw.Spec.Days,
			MonitorIDs:      monitorIDsPtr,
		}
		// Only include date for "once" interval (API rejects it for daily/weekly/monthly)
		if mw.Spec.Interval == intervalOnce {
			updateReq.Date = mw.Spec.StartDate
		}

		result, err := urclient.UpdateMaintenanceWindow(ctx, mw.Status.ID, updateReq)
		if err != nil {
			// If maintenance window was deleted out-of-band, recreate it
			if uptimerobot.IsNotFound(err) {
				logger.Info("Maintenance window not found in UptimeRobot, recreating", "id", mw.Status.ID)
				var recreateMonitorIDs []int
				if !mw.Spec.AutoAddMonitors {
					recreateMonitorIDs = monitorIDs
				}
				createReq := uptimerobot.CreateMaintenanceWindowRequest{
					Name:            mw.Spec.Name,
					AutoAddMonitors: mw.Spec.AutoAddMonitors,
					Interval:        mw.Spec.Interval,
					Time:            mw.Spec.StartTime,
					Duration:        durationMinutes,
					Days:            mw.Spec.Days,
					MonitorIDs:      recreateMonitorIDs,
				}
				// Only include date for "once" interval
				if mw.Spec.Interval == intervalOnce {
					createReq.Date = mw.Spec.StartDate
				}

				result, err := urclient.CreateMaintenanceWindow(ctx, createReq)
				if err != nil {
					mw.Status.Ready = false
					msg := fmt.Sprintf("Failed to recreate maintenance window: %v", err)
					SetReadyCondition(&mw.Status.Conditions, false, ReasonAPIError, msg, mw.Generation)
					SetSyncedCondition(&mw.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to sync with UptimeRobot: %v", err), mw.Generation)
					SetErrorCondition(&mw.Status.Conditions, true, ReasonAPIError, msg, mw.Generation)
					if updateErr := r.Status().Update(ctx, mw); updateErr != nil {
						return ctrl.Result{}, updateErr
					}
					return ctrl.Result{}, fmt.Errorf("failed to recreate maintenance window: %w", err)
				}

				mw.Status.ID = strconv.Itoa(result.ID)
				mw.Status.MonitorCount = len(result.MonitorIDs)
				SetReadyCondition(&mw.Status.Conditions, true, ReasonReconcileSuccess, "MaintenanceWindow reconciled successfully", mw.Generation)
				SetSyncedCondition(&mw.Status.Conditions, true, ReasonSyncSuccess, "Successfully synced with UptimeRobot", mw.Generation)
				SetErrorCondition(&mw.Status.Conditions, false, ReasonReconcileSuccess, "", mw.Generation)
				if err := r.Status().Update(ctx, mw); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{RequeueAfter: mw.Spec.SyncInterval.Duration}, nil
			}
			msg := fmt.Sprintf("Failed to update maintenance window: %v", err)
			SetReadyCondition(&mw.Status.Conditions, false, ReasonAPIError, msg, mw.Generation)
			SetSyncedCondition(&mw.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to sync with UptimeRobot: %v", err), mw.Generation)
			SetErrorCondition(&mw.Status.Conditions, true, ReasonAPIError, msg, mw.Generation)
			if updateErr := r.Status().Update(ctx, mw); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, fmt.Errorf("failed to update maintenance window: %w", err)
		}

		mw.Status.MonitorCount = len(result.MonitorIDs)
		SetReadyCondition(&mw.Status.Conditions, true, ReasonReconcileSuccess, "MaintenanceWindow reconciled successfully", mw.Generation)
		SetSyncedCondition(&mw.Status.Conditions, true, ReasonSyncSuccess, "Successfully synced with UptimeRobot", mw.Generation)
		SetErrorCondition(&mw.Status.Conditions, false, ReasonReconcileSuccess, "", mw.Generation)
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
