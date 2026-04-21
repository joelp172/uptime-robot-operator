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
	"time"

	"github.com/joelp172/uptime-robot-operator/internal/metrics"
	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MonitorGroupReconciler orchestrates MonitorGroup lifecycle
type MonitorGroupReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=uptimerobot.com,resources=monitorgroups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=uptimerobot.com,resources=monitorgroups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=uptimerobot.com,resources=monitorgroups/finalizers,verbs=update
//+kubebuilder:rbac:groups=uptimerobot.com,resources=monitors,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile implements the reconciliation loop
func (r *MonitorGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.ReconciliationDuration.WithLabelValues("monitorgroup").Observe(duration)
	}()

	logger := log.FromContext(ctx)

	// Step 1: Retrieve the resource from cluster
	groupResource := &uptimerobotv1.MonitorGroup{}
	if fetchErr := r.Get(ctx, req.NamespacedName, groupResource); fetchErr != nil {
		return ctrl.Result{}, client.IgnoreNotFound(fetchErr)
	}

	// Update observedGeneration
	groupResource.Status.ObservedGeneration = groupResource.Generation

	// Step 2: Handle deletion workflow first so missing account/secret does not block finalization.
	const cleanupMarker = "uptimerobot.com/finalizer"
	if !groupResource.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(groupResource, cleanupMarker) {
			// Initialize cleanup tracking on first deletion attempt
			if err := InitializeCleanupTracking(ctx, r.Client, groupResource); err != nil {
				return ctrl.Result{}, err
			}

			// Reload the monitor group to get the updated annotations
			if err := r.Get(ctx, req.NamespacedName, groupResource); err != nil {
				return ctrl.Result{}, client.IgnoreNotFound(err)
			}

			// Define the cleanup function
			cleanupFunc := func(ctx context.Context) error {
				if !groupResource.Spec.Prune || groupResource.Status.ID == "" {
					// Skip cleanup if Prune is false or resource has no backend ID
					return nil
				}
				credentialVault := &uptimerobotv1.Account{}
				if vaultErr := GetAccount(ctx, r.Client, credentialVault, groupResource.Spec.Account.Name); vaultErr != nil {
					return fmt.Errorf("failed to get account for cleanup: %w", vaultErr)
				}
				apiToken, tokenErr := GetApiKey(ctx, r.Client, credentialVault)
				if tokenErr != nil {
					return fmt.Errorf("failed to get api key for cleanup: %w", tokenErr)
				}
				backendClient := uptimerobot.NewClient(apiToken)
				return backendClient.PurgeGroupFromBackend(ctx, groupResource.Status.ID)
			}

			// Handle cleanup with retry and timeout logic
			result, _ := HandleFinalizerCleanup(ctx, CleanupOptions{
				Object:             groupResource,
				Conditions:         &groupResource.Status.Conditions,
				ObservedGeneration: groupResource.Generation,
				Recorder:           r.Recorder,
				CleanupFunc:        cleanupFunc,
			})

			// Update status with cleanup progress
			if updateErr := r.updateMonitorGroupStatus(ctx, groupResource); updateErr != nil {
				return ctrl.Result{}, updateErr
			}

			// If cleanup failed and we shouldn't force-remove, requeue
			if !result.Success && !result.ForceRemove {
				return ctrl.Result{RequeueAfter: result.RequeueAfter}, nil
			}

			// Successful cleanup API call confirms the API is healthy.
			if result.Success {
				cleanupAccount := &uptimerobotv1.Account{}
				if err := GetAccount(ctx, r.Client, cleanupAccount, groupResource.Spec.Account.Name); err == nil {
					onAPISuccess(cleanupAccount.Name, r.Recorder, groupResource)
				}
			}

			// Remove finalizer (either success or force-remove)
			controllerutil.RemoveFinalizer(groupResource, cleanupMarker)
			if updateErr := r.Update(ctx, groupResource); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
		}

		return ctrl.Result{}, nil
	}

	// Step 3: Locate credential vault
	credentialVault := &uptimerobotv1.Account{}
	if vaultErr := GetAccount(ctx, r.Client, credentialVault, groupResource.Spec.Account.Name); vaultErr != nil {
		metrics.ReconciliationErrorsTotal.WithLabelValues("monitorgroup", "account_not_found").Inc()
		if groupResource.Status.ID == "" {
			groupResource.Status.Ready = false
		}
		msg := fmt.Sprintf("Failed to get account: %v", vaultErr)
		SetReadyCondition(&groupResource.Status.Conditions, false, ReasonReconcileError, msg, groupResource.Generation)
		SetErrorCondition(&groupResource.Status.Conditions, true, ReasonReconcileError, msg, groupResource.Generation)
		if r.Recorder != nil {
			r.Recorder.Event(groupResource, "Warning", "DependencyNotReady", msg)
		}
		if updateErr := r.updateMonitorGroupStatus(ctx, groupResource); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, vaultErr
	}

	// Step 4: Extract API token from secret
	apiToken, tokenErr := GetApiKey(ctx, r.Client, credentialVault)
	if tokenErr != nil {
		metrics.ReconciliationErrorsTotal.WithLabelValues("monitorgroup", "secret_not_found").Inc()
		if groupResource.Status.ID == "" {
			groupResource.Status.Ready = false
		}
		msg := fmt.Sprintf("Failed to get API key: %v", tokenErr)
		SetReadyCondition(&groupResource.Status.Conditions, false, ReasonSecretNotFound, msg, groupResource.Generation)
		SetErrorCondition(&groupResource.Status.Conditions, true, ReasonSecretNotFound, msg, groupResource.Generation)
		if r.Recorder != nil {
			r.Recorder.Event(groupResource, "Warning", "SecretNotFound", msg)
		}
		if updateErr := r.updateMonitorGroupStatus(ctx, groupResource); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, tokenErr
	}
	backendClient := uptimerobot.NewClient(apiToken)
	accountKey := credentialVault.Name

	// Circuit breaker: skip API calls when the circuit is not allowing traffic.
	if !DefaultCircuitBreaker.Allow(accountKey) {
		state := DefaultCircuitBreaker.State(accountKey)
		logger.Info("Circuit breaker blocking API call", "account", accountKey, "state", state)
		cooldown := DefaultCircuitBreaker.CooldownPeriod()
		reason, eventReason, msg := circuitBreakerBlockDetails(accountKey, state, cooldown)
		alreadyBlocked := conditionMatches(groupResource.Status.Conditions, TypeSynced, metav1.ConditionFalse, reason, msg) &&
			conditionMatches(groupResource.Status.Conditions, TypeReady, metav1.ConditionFalse, reason, msg) &&
			conditionMatches(groupResource.Status.Conditions, TypeError, metav1.ConditionTrue, reason, msg)
		if !alreadyBlocked {
			SetReadyCondition(&groupResource.Status.Conditions, false, reason, msg, groupResource.Generation)
			SetSyncedCondition(&groupResource.Status.Conditions, false, reason, msg, groupResource.Generation)
			SetErrorCondition(&groupResource.Status.Conditions, true, reason, msg, groupResource.Generation)
			if r.Recorder != nil {
				r.Recorder.Event(groupResource, "Warning", eventReason, msg)
			}
			if updateErr := r.updateMonitorGroupStatus(ctx, groupResource); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
		}
		return ctrl.Result{RequeueAfter: cooldown}, nil
	}

	// Step 5: Attach finalizer for cleanup tracking
	if !controllerutil.ContainsFinalizer(groupResource, cleanupMarker) {
		controllerutil.AddFinalizer(groupResource, cleanupMarker)
		if updateErr := r.Update(ctx, groupResource); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
	}

	// Step 6: Resolve monitor IDs from local references
	aggregatedMonitorIDs := []int{}
	for _, monitorLink := range groupResource.Spec.Monitors {
		if monitorLink.Name == "" {
			continue
		}

		monitorInstance := &uptimerobotv1.Monitor{}
		lookupKey := types.NamespacedName{
			Namespace: req.Namespace,
			Name:      monitorLink.Name,
		}

		if lookupErr := r.Get(ctx, lookupKey, monitorInstance); lookupErr != nil {
			logger.Info("Monitor reference unresolved, skipping", "monitor", monitorLink.Name, "error", lookupErr)
			continue
		}

		if !monitorInstance.Status.Ready || monitorInstance.Status.ID == "" {
			logger.Info("Monitor not yet operational, skipping", "monitor", monitorLink.Name)
			continue
		}

		monitorNumericID, conversionErr := strconv.Atoi(monitorInstance.Status.ID)
		if conversionErr != nil {
			logger.Info("Monitor ID conversion failed, skipping", "monitor", monitorLink.Name, "id", monitorInstance.Status.ID)
			continue
		}

		aggregatedMonitorIDs = append(aggregatedMonitorIDs, monitorNumericID)
	}

	// Step 7: Execute creation or update logic
	if groupResource.Status.ID == "" {
		// Creation pathway
		creationPayload := uptimerobot.GroupCreationWireFormat{
			Name:       groupResource.Spec.FriendlyName,
			MonitorIDs: aggregatedMonitorIDs,
			GroupIDs:   groupResource.Spec.PullFromGroups,
		}

		backendResponse, creationErr := backendClient.SpawnGroupInBackend(ctx, creationPayload)
		if creationErr != nil {
			metrics.ReconciliationErrorsTotal.WithLabelValues("monitorgroup", "api_error").Inc()
			groupResource.Status.Ready = false
			msg := fmt.Sprintf("Group creation failed: %v", creationErr)
			SetReadyCondition(&groupResource.Status.Conditions, false, ReasonAPIError, msg, groupResource.Generation)
			SetSyncedCondition(&groupResource.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to create group in UptimeRobot: %v", creationErr), groupResource.Generation)
			SetErrorCondition(&groupResource.Status.Conditions, true, ReasonAPIError, msg, groupResource.Generation)
			if r.Recorder != nil {
				r.Recorder.Event(groupResource, "Warning", "SyncFailed", msg)
			}
			onAPIFailure(accountKey, creationErr, r.Recorder, groupResource)
			if updateErr := r.updateMonitorGroupStatus(ctx, groupResource); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, fmt.Errorf("group creation failed: %w", creationErr)
		}

		groupResource.Status.Ready = true
		groupResource.Status.ID = strconv.Itoa(backendResponse.ID)
		groupResource.Status.MonitorCount = len(aggregatedMonitorIDs)
		nowTimestamp := metav1.Now()
		groupResource.Status.LastReconciled = &nowTimestamp
		SetReadyCondition(&groupResource.Status.Conditions, true, ReasonReconcileSuccess, "MonitorGroup reconciled successfully", groupResource.Generation)
		SetSyncedCondition(&groupResource.Status.Conditions, true, ReasonSyncSuccess, "Successfully synced with UptimeRobot", groupResource.Generation)
		SetErrorCondition(&groupResource.Status.Conditions, false, ReasonReconcileSuccess, "", groupResource.Generation)

		if r.Recorder != nil {
			r.Recorder.Event(groupResource, "Normal", "Created", fmt.Sprintf("Monitor group created with ID %s", groupResource.Status.ID))
		}

		onAPISuccess(accountKey, r.Recorder, groupResource)
		if statusErr := r.updateMonitorGroupStatus(ctx, groupResource); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
	} else {
		// Update pathway
		updatePayload := uptimerobot.GroupUpdateWireFormat{
			Name:       groupResource.Spec.FriendlyName,
			MonitorIDs: &aggregatedMonitorIDs,
			GroupIDs:   groupResource.Spec.PullFromGroups,
		}

		_, updateErr := backendClient.MutateGroupInBackend(ctx, groupResource.Status.ID, updatePayload)
		if updateErr != nil {
			// Handle out-of-band deletion
			if uptimerobot.IsNotFound(updateErr) {
				logger.Info("Group missing from backend, recreating", "id", groupResource.Status.ID)

				creationPayload := uptimerobot.GroupCreationWireFormat{
					Name:       groupResource.Spec.FriendlyName,
					MonitorIDs: aggregatedMonitorIDs,
					GroupIDs:   groupResource.Spec.PullFromGroups,
				}

				backendResponse, recreationErr := backendClient.SpawnGroupInBackend(ctx, creationPayload)
				if recreationErr != nil {
					metrics.ReconciliationErrorsTotal.WithLabelValues("monitorgroup", "api_error").Inc()
					groupResource.Status.Ready = false
					msg := fmt.Sprintf("Group recreation failed: %v", recreationErr)
					SetReadyCondition(&groupResource.Status.Conditions, false, ReasonAPIError, msg, groupResource.Generation)
					SetSyncedCondition(&groupResource.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to recreate group in UptimeRobot: %v", recreationErr), groupResource.Generation)
					SetErrorCondition(&groupResource.Status.Conditions, true, ReasonAPIError, msg, groupResource.Generation)
					if r.Recorder != nil {
						r.Recorder.Event(groupResource, "Warning", "SyncFailed", msg)
					}
					onAPIFailure(accountKey, recreationErr, r.Recorder, groupResource)
					if updateErr := r.updateMonitorGroupStatus(ctx, groupResource); updateErr != nil {
						return ctrl.Result{}, updateErr
					}
					return ctrl.Result{}, fmt.Errorf("group recreation failed: %w", recreationErr)
				}

				groupResource.Status.ID = strconv.Itoa(backendResponse.ID)
				groupResource.Status.MonitorCount = len(aggregatedMonitorIDs)
				nowTimestamp := metav1.Now()
				groupResource.Status.LastReconciled = &nowTimestamp
				SetReadyCondition(&groupResource.Status.Conditions, true, ReasonReconcileSuccess, "MonitorGroup reconciled successfully", groupResource.Generation)
				SetSyncedCondition(&groupResource.Status.Conditions, true, ReasonSyncSuccess, "Successfully synced with UptimeRobot", groupResource.Generation)
				SetErrorCondition(&groupResource.Status.Conditions, false, ReasonReconcileSuccess, "", groupResource.Generation)

				if r.Recorder != nil {
					r.Recorder.Event(groupResource, "Normal", "Recreated", fmt.Sprintf("Monitor group recreated with ID %s", groupResource.Status.ID))
				}

				onAPISuccess(accountKey, r.Recorder, groupResource)
				if statusErr := r.updateMonitorGroupStatus(ctx, groupResource); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				return ctrl.Result{RequeueAfter: AddSyncJitter(groupResource.Spec.SyncInterval.Duration)}, nil
			}
			metrics.ReconciliationErrorsTotal.WithLabelValues("monitorgroup", "api_error").Inc()
			msg := fmt.Sprintf("Group update failed: %v", updateErr)
			SetReadyCondition(&groupResource.Status.Conditions, false, ReasonAPIError, msg, groupResource.Generation)
			SetSyncedCondition(&groupResource.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to update group in UptimeRobot: %v", updateErr), groupResource.Generation)
			SetErrorCondition(&groupResource.Status.Conditions, true, ReasonAPIError, msg, groupResource.Generation)
			if r.Recorder != nil {
				r.Recorder.Event(groupResource, "Warning", "SyncFailed", msg)
			}
			onAPIFailure(accountKey, updateErr, r.Recorder, groupResource)
			if statusUpdateErr := r.updateMonitorGroupStatus(ctx, groupResource); statusUpdateErr != nil {
				return ctrl.Result{}, statusUpdateErr
			}
			return ctrl.Result{}, fmt.Errorf("group update failed: %w", updateErr)
		}

		groupResource.Status.MonitorCount = len(aggregatedMonitorIDs)
		nowTimestamp := metav1.Now()
		groupResource.Status.LastReconciled = &nowTimestamp
		SetReadyCondition(&groupResource.Status.Conditions, true, ReasonReconcileSuccess, "MonitorGroup reconciled successfully", groupResource.Generation)
		SetSyncedCondition(&groupResource.Status.Conditions, true, ReasonSyncSuccess, "Successfully synced with UptimeRobot", groupResource.Generation)
		SetErrorCondition(&groupResource.Status.Conditions, false, ReasonReconcileSuccess, "", groupResource.Generation)

		if r.Recorder != nil {
			r.Recorder.Event(groupResource, "Normal", "Updated", "Monitor group updated successfully")
		}

		onAPISuccess(accountKey, r.Recorder, groupResource)
		if statusErr := r.updateMonitorGroupStatus(ctx, groupResource); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
	}

	return ctrl.Result{RequeueAfter: AddSyncJitter(groupResource.Spec.SyncInterval.Duration)}, nil
}

// updateMonitorGroupStatus writes status and retries on conflict by refetching
// the latest resource and re-applying the desired status, so a successful
// create isn't followed by another create attempt when the first status write
// loses a resourceVersion race.
func (r *MonitorGroupReconciler) updateMonitorGroupStatus(ctx context.Context, groupResource *uptimerobotv1.MonitorGroup) error {
	err := r.Status().Update(ctx, groupResource)
	if err == nil {
		return nil
	}
	if !apierrors.IsConflict(err) {
		return err
	}

	log.FromContext(ctx).Info("status update conflicted, refetching and retrying", "monitorgroup", client.ObjectKeyFromObject(groupResource))
	desiredStatus := groupResource.Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &uptimerobotv1.MonitorGroup{}
		if getErr := r.Get(ctx, client.ObjectKeyFromObject(groupResource), latest); getErr != nil {
			return fmt.Errorf("refetch for status retry: %w", getErr)
		}
		latest.Status = desiredStatus
		return r.Status().Update(ctx, latest)
	})
}

// SetupWithManager configures the controller with manager
func (r *MonitorGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Establish indexing for efficient monitor reference lookups
	if indexErr := mgr.GetFieldIndexer().IndexField(context.Background(), &uptimerobotv1.MonitorGroup{}, "spec.monitors", func(obj client.Object) []string {
		groupResource := obj.(*uptimerobotv1.MonitorGroup)
		var monitorNames []string
		for _, monitorLink := range groupResource.Spec.Monitors {
			if monitorLink.Name != "" {
				monitorNames = append(monitorNames, monitorLink.Name)
			}
		}
		return monitorNames
	}); indexErr != nil {
		return indexErr
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&uptimerobotv1.MonitorGroup{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(
			&uptimerobotv1.Monitor{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				monitorInstance := obj.(*uptimerobotv1.Monitor)

				// Discover MonitorGroups referencing this Monitor
				var groupCollection uptimerobotv1.MonitorGroupList
				if listErr := r.List(ctx, &groupCollection,
					client.InNamespace(monitorInstance.Namespace),
					client.MatchingFields{"spec.monitors": monitorInstance.Name},
				); listErr != nil {
					return nil
				}

				// Generate reconcile requests
				requests := make([]reconcile.Request, len(groupCollection.Items))
				for i, groupResource := range groupCollection.Items {
					requests[i] = reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: groupResource.Namespace,
							Name:      groupResource.Name,
						},
					}
				}
				return requests
			}),
		).
		Named("monitorgroup").
		Complete(r)
}
