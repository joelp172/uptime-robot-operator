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
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot/urtypes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

// MonitorReconciler reconciles a Monitor object
type MonitorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	// AdoptIDAnnotation is used to specify an existing monitor ID to adopt
	AdoptIDAnnotation       = "uptimerobot.com/adopt-id"
	defaultHeartbeatBaseURL = "https://heartbeat.uptimerobot.com"
	heartbeatBaseURLEnvVar  = "UPTIMEROBOT_HEARTBEAT_BASE_URL"
)

var (
	ErrContactMissingID = errors.New("contact missing ID")
	ErrSecretMissingKey = errors.New("secret missing key")
)

//+kubebuilder:rbac:groups=uptimerobot.com,resources=monitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=uptimerobot.com,resources=monitors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=uptimerobot.com,resources=monitors/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *MonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	monitor := &uptimerobotv1.Monitor{}
	if err := r.Get(ctx, req.NamespacedName, monitor); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	account := &uptimerobotv1.Account{}
	if err := GetAccount(ctx, r.Client, account, monitor.Spec.Account.Name); err != nil {
		return ctrl.Result{}, err
	}

	apiKey, err := GetApiKey(ctx, r.Client, account)
	if err != nil {
		return ctrl.Result{}, err
	}
	urclient := uptimerobot.NewClient(apiKey)

	const myFinalizerName = "uptimerobot.com/finalizer"
	if !monitor.DeletionTimestamp.IsZero() {
		// Object is being deleted
		if controllerutil.ContainsFinalizer(monitor, myFinalizerName) {
			if monitor.Spec.Prune && monitor.Status.Ready {
				// Check if another Monitor resource has adopted this monitor ID
				// If so, don't delete it from UptimeRobot
				shouldDelete := true
				if monitor.Status.ID != "" {
					// List monitors in the same namespace to check for adopters
					// Scoping to namespace prevents false positives from other namespaces
					var namespaceMonitors uptimerobotv1.MonitorList
					if err := r.List(ctx, &namespaceMonitors, client.InNamespace(monitor.Namespace)); err != nil {
						return ctrl.Result{}, err
					}
					for i := range namespaceMonitors.Items {
						otherMonitor := &namespaceMonitors.Items[i]
						// Skip the current monitor being deleted
						if otherMonitor.UID == monitor.UID {
							continue
						}
						// Skip monitors that are themselves being deleted
						if !otherMonitor.DeletionTimestamp.IsZero() {
							continue
						}
						// Skip monitors using a different account (different UptimeRobot accounts can have same IDs)
						if otherMonitor.Spec.Account.Name != monitor.Spec.Account.Name {
							continue
						}

						// Check if another monitor has adopted this ID
						// A monitor is considered an adopter if:
						// 1. It has the adopt-id annotation matching this monitor's ID (intent to adopt)
						// 2. OR it has status.ready=true and status.id matching this monitor's ID (successfully adopted)
						isAdopter := false
						if adoptID, hasAdoptID := otherMonitor.Annotations[AdoptIDAnnotation]; hasAdoptID && adoptID == monitor.Status.ID {
							// Has adopt-id annotation - intent to adopt
							isAdopter = true
						} else if otherMonitor.Status.Ready && otherMonitor.Status.ID == monitor.Status.ID {
							// Successfully adopted (status.ready=true and same ID)
							isAdopter = true
						}

						if isAdopter {
							log.FromContext(ctx).Info("Monitor ID is managed by another resource, skipping deletion from UptimeRobot",
								"monitorID", monitor.Status.ID,
								"otherMonitor", otherMonitor.Name,
								"otherNamespace", otherMonitor.Namespace,
								"otherAccount", otherMonitor.Spec.Account.Name)
							shouldDelete = false
							break
						}
					}
				}

				if shouldDelete {
					if err := urclient.DeleteMonitor(ctx, monitor.Status.ID); err != nil {
						return ctrl.Result{}, err
					}
				}
			}

			controllerutil.RemoveFinalizer(monitor, myFinalizerName)
			if err := r.Update(ctx, monitor); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(monitor, myFinalizerName) {
		controllerutil.AddFinalizer(monitor, myFinalizerName)
		if err := r.Update(ctx, monitor); err != nil {
			return ctrl.Result{}, err
		}
	}

	if monitor.Status.Ready && monitor.Status.Type != monitor.Spec.Monitor.Type {
		// Type change requires recreate
		if err := urclient.DeleteMonitor(ctx, monitor.Status.ID); err != nil {
			monitor.Status.Ready = false
			SetReadyCondition(&monitor.Status.Conditions, false, ReasonAPIError, fmt.Sprintf("Failed to delete monitor for type change: %v", err), monitor.Generation)
			SetSyncedCondition(&monitor.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to delete monitor for type change: %v", err), monitor.Generation)
			SetErrorCondition(&monitor.Status.Conditions, true, ReasonAPIError, err.Error(), monitor.Generation)
			if updateErr := r.updateMonitorStatus(ctx, monitor); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, err
		}
		monitor.Status.Ready = false
	}

	contacts := make([]uptimerobotv1.MonitorContact, 0, len(monitor.Spec.Contacts))
	for _, ref := range monitor.Spec.Contacts {
		contact := &uptimerobotv1.Contact{}

		if err := GetContact(ctx, r.Client, contact, ref.Name); err != nil {
			return ctrl.Result{}, err
		}

		if contact.Status.ID == "" {
			// Contact hasn't been reconciled yet - requeue without error
			log.FromContext(ctx).Info("Contact not ready yet, requeuing", "contact", ref.Name)
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}

		contacts = append(contacts, uptimerobotv1.MonitorContact{
			ID:                   contact.Status.ID,
			MonitorContactCommon: ref.MonitorContactCommon,
		})
	}

	if auth := monitor.Spec.Monitor.Auth; auth != nil && auth.SecretName != "" {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: req.Namespace,
			Name:      auth.SecretName,
		}, secret); err != nil {
			return ctrl.Result{}, err
		}

		val, ok := secret.Data[auth.UsernameKey]
		if !ok {
			return ctrl.Result{}, fmt.Errorf("%w: %s", ErrSecretMissingKey, auth.UsernameKey)
		}
		auth.Username = string(val)

		if auth.PasswordKey != "" {
			val, ok := secret.Data[auth.PasswordKey]
			if !ok {
				return ctrl.Result{}, fmt.Errorf("%w: %s", ErrSecretMissingKey, auth.PasswordKey)
			}
			auth.Password = string(val)
		}
	}

	if !monitor.Status.Ready {
		// Check if adoption is requested via annotation
		if adoptID, hasAdoptID := monitor.Annotations[AdoptIDAnnotation]; hasAdoptID && adoptID != "" {
			// Adopt existing monitor
			log.FromContext(ctx).Info("Adopting existing monitor", "monitorID", adoptID)

			// Verify monitor exists
			existingMonitor, err := urclient.GetMonitor(ctx, adoptID)
			if err != nil {
				monitor.Status.Ready = false
				if errors.Is(err, uptimerobot.ErrMonitorNotFound) {
					msg := fmt.Sprintf("cannot adopt monitor: monitor with ID %s not found", adoptID)
					// This is validation during adoption - treat same as type mismatch validation
					SetReadyCondition(&monitor.Status.Conditions, false, ReasonReconcileError, msg, monitor.Generation)
					SetErrorCondition(&monitor.Status.Conditions, true, ReasonReconcileError, msg, monitor.Generation)
					if updateErr := r.updateMonitorStatus(ctx, monitor); updateErr != nil {
						return ctrl.Result{}, updateErr
					}
					return ctrl.Result{}, errors.New(msg)
				}
				msg := fmt.Sprintf("failed to get monitor for adoption: %v", err)
				SetReadyCondition(&monitor.Status.Conditions, false, ReasonAPIError, msg, monitor.Generation)
				SetSyncedCondition(&monitor.Status.Conditions, false, ReasonSyncError, msg, monitor.Generation)
				SetErrorCondition(&monitor.Status.Conditions, true, ReasonAPIError, err.Error(), monitor.Generation)
				if updateErr := r.updateMonitorStatus(ctx, monitor); updateErr != nil {
					return ctrl.Result{}, updateErr
				}
				return ctrl.Result{}, fmt.Errorf("failed to get monitor for adoption: %w", err)
			}

			// Verify monitor type matches spec
			existingType := urtypes.MonitorTypeFromAPIString(existingMonitor.Type)
			if existingType != monitor.Spec.Monitor.Type {
				monitor.Status.Ready = false
				msg := fmt.Sprintf("cannot adopt monitor: type mismatch - existing monitor is %s but spec defines %s", existingType.String(), monitor.Spec.Monitor.Type.String())
				// Don't set Synced here since this is a validation error before sync attempt
				SetReadyCondition(&monitor.Status.Conditions, false, ReasonReconcileError, msg, monitor.Generation)
				SetErrorCondition(&monitor.Status.Conditions, true, ReasonReconcileError, msg, monitor.Generation)
				if updateErr := r.updateMonitorStatus(ctx, monitor); updateErr != nil {
					return ctrl.Result{}, updateErr
				}
				return ctrl.Result{}, errors.New(msg)
			}

			// Adopt the monitor by setting status
			monitor.Status.Ready = true
			monitor.Status.ID = adoptID
			monitor.Status.Type = monitor.Spec.Monitor.Type
			monitor.Status.Status = monitor.Spec.Monitor.Status
			// Set HeartbeatURL for heartbeat monitors
			if monitor.Spec.Monitor.Type == urtypes.TypeHeartbeat && existingMonitor.URL != "" {
				monitor.Status.HeartbeatURL = buildHeartbeatURL(configuredHeartbeatBaseURL(), monitor.Status.ID, existingMonitor.URL)
			}
			if err := r.updateMonitorStatus(ctx, monitor); err != nil {
				return ctrl.Result{}, err
			}

			// Apply the spec to the adopted monitor immediately
			result, err := urclient.EditMonitor(ctx, adoptID, monitor.Spec.Monitor, contacts)
			if err != nil {
				monitor.Status.Ready = false
				SetReadyCondition(&monitor.Status.Conditions, false, ReasonAPIError, fmt.Sprintf("Failed to edit adopted monitor: %v", err), monitor.Generation)
				SetSyncedCondition(&monitor.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to edit adopted monitor: %v", err), monitor.Generation)
				SetErrorCondition(&monitor.Status.Conditions, true, ReasonAPIError, err.Error(), monitor.Generation)
				if updateErr := r.updateMonitorStatus(ctx, monitor); updateErr != nil {
					return ctrl.Result{}, updateErr
				}
				return ctrl.Result{}, err
			}
			// Update status with any changes from the edit (e.g., heartbeat URL)
			monitor.Status.ID = result.ID
			monitor.Status.Status = monitor.Spec.Monitor.Status
			if monitor.Spec.Monitor.Type == urtypes.TypeHeartbeat && result.URL != "" {
				monitor.Status.HeartbeatURL = buildHeartbeatURL(configuredHeartbeatBaseURL(), monitor.Status.ID, result.URL)
			}
			if err := r.updateMonitorStatus(ctx, monitor); err != nil {
				return ctrl.Result{}, err
			}

			log.FromContext(ctx).Info("Successfully adopted and updated monitor", "monitorID", adoptID)
		} else {
			// Create new monitor
			result, err := urclient.CreateMonitor(ctx, monitor.Spec.Monitor, contacts)
			if err != nil {
				monitor.Status.Ready = false
				SetReadyCondition(&monitor.Status.Conditions, false, ReasonAPIError, fmt.Sprintf("Failed to create monitor: %v", err), monitor.Generation)
				SetSyncedCondition(&monitor.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to create monitor: %v", err), monitor.Generation)
				SetErrorCondition(&monitor.Status.Conditions, true, ReasonAPIError, err.Error(), monitor.Generation)
				if updateErr := r.updateMonitorStatus(ctx, monitor); updateErr != nil {
					return ctrl.Result{}, updateErr
				}
				return ctrl.Result{}, err
			}

			monitor.Status.Ready = true
			monitor.Status.ID = result.ID
			monitor.Status.Type = monitor.Spec.Monitor.Type
			monitor.Status.Status = monitor.Spec.Monitor.Status
			// Set HeartbeatURL for heartbeat monitors (API returns token, we need full URL)
			if monitor.Spec.Monitor.Type == urtypes.TypeHeartbeat && result.URL != "" {
				monitor.Status.HeartbeatURL = buildHeartbeatURL(configuredHeartbeatBaseURL(), monitor.Status.ID, result.URL)
			}
			if err := r.updateMonitorStatus(ctx, monitor); err != nil {
				return ctrl.Result{}, err
			}

			if monitor.Spec.Monitor.Status == urtypes.MonitorPaused {
				if _, err := urclient.EditMonitor(ctx, result.ID, monitor.Spec.Monitor, contacts); err != nil {
					monitor.Status.Ready = false
					SetReadyCondition(&monitor.Status.Conditions, false, ReasonAPIError, fmt.Sprintf("Failed to pause monitor: %v", err), monitor.Generation)
					SetSyncedCondition(&monitor.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to pause monitor: %v", err), monitor.Generation)
					SetErrorCondition(&monitor.Status.Conditions, true, ReasonAPIError, err.Error(), monitor.Generation)
					if updateErr := r.updateMonitorStatus(ctx, monitor); updateErr != nil {
						return ctrl.Result{}, updateErr
					}
					return ctrl.Result{}, err
				}
			}
		}
	} else {
		result, err := urclient.EditMonitor(ctx, monitor.Status.ID, monitor.Spec.Monitor, contacts)
		if err != nil {
			monitor.Status.Ready = false
			SetReadyCondition(&monitor.Status.Conditions, false, ReasonAPIError, fmt.Sprintf("Failed to edit monitor: %v", err), monitor.Generation)
			SetSyncedCondition(&monitor.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to edit monitor: %v", err), monitor.Generation)
			SetErrorCondition(&monitor.Status.Conditions, true, ReasonAPIError, err.Error(), monitor.Generation)
			if updateErr := r.updateMonitorStatus(ctx, monitor); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, err
		}

		monitor.Status.ID = result.ID
		monitor.Status.Status = monitor.Spec.Monitor.Status
		// Update HeartbeatURL for heartbeat monitors (API returns token, we need full URL)
		if monitor.Spec.Monitor.Type == urtypes.TypeHeartbeat && result.URL != "" {
			monitor.Status.HeartbeatURL = buildHeartbeatURL(configuredHeartbeatBaseURL(), monitor.Status.ID, result.URL)
		}
		if err := r.updateMonitorStatus(ctx, monitor); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileHeartbeatURLPublishTarget(ctx, monitor); err != nil {
		monitor.Status.Ready = false
		// Note: We don't set Synced=false here because the monitor successfully synced with UptimeRobot.
		// Only the local heartbeat URL publishing failed, which doesn't affect the sync status.
		// The Synced condition will retain its previous successful state (true).
		SetReadyCondition(&monitor.Status.Conditions, false, ReasonReconcileError, fmt.Sprintf("Failed to reconcile heartbeat URL publish target: %v", err), monitor.Generation)
		SetErrorCondition(&monitor.Status.Conditions, true, ReasonReconcileError, err.Error(), monitor.Generation)
		if updateErr := r.updateMonitorStatus(ctx, monitor); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	monitor.Status.ObservedGeneration = monitor.Generation
	SetReadyCondition(&monitor.Status.Conditions, true, ReasonReconcileSuccess, "Monitor reconciled successfully", monitor.Generation)
	SetSyncedCondition(&monitor.Status.Conditions, true, ReasonSyncSuccess, "Successfully synced with UptimeRobot", monitor.Generation)
	SetErrorCondition(&monitor.Status.Conditions, false, ReasonReconcileSuccess, "", monitor.Generation)

	return ctrl.Result{RequeueAfter: monitor.Spec.SyncInterval.Duration}, nil
}

func configuredHeartbeatBaseURL() string {
	return normalizeHeartbeatBaseURL(os.Getenv(heartbeatBaseURLEnvVar))
}

func normalizeHeartbeatBaseURL(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return defaultHeartbeatBaseURL
	}
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		trimmed = "https://" + trimmed
	}
	return strings.TrimSuffix(trimmed, "/")
}

func buildHeartbeatURL(baseURL, monitorID, apiURL string) string {
	trimmed := strings.TrimSpace(apiURL)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	normalizedBaseURL := normalizeHeartbeatBaseURL(baseURL)
	if isPrefixedHeartbeatKey(trimmed) {
		return fmt.Sprintf("%s/%s", normalizedBaseURL, trimmed)
	}
	if monitorID != "" {
		return fmt.Sprintf("%s/m%s-%s", normalizedBaseURL, monitorID, trimmed)
	}
	return fmt.Sprintf("%s/%s", normalizedBaseURL, trimmed)
}

func isPrefixedHeartbeatKey(value string) bool {
	if len(value) < 3 {
		return false
	}
	if value[0] != 'm' {
		return false
	}
	rest := value[1:]
	dashIdx := strings.IndexByte(rest, '-')
	if dashIdx <= 0 {
		return false
	}
	for _, c := range rest[:dashIdx] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (r *MonitorReconciler) reconcileHeartbeatURLPublishTarget(ctx context.Context, monitor *uptimerobotv1.Monitor) error {
	currentTargetType := monitor.Status.HeartbeatURLPublishTargetType
	currentTargetName := strings.TrimSpace(monitor.Status.HeartbeatURLPublishTargetName)
	currentTargetKey := strings.TrimSpace(monitor.Status.HeartbeatURLPublishTargetKey)

	publish := monitor.Spec.HeartbeatURLPublish
	heartbeatURL := buildHeartbeatURL(configuredHeartbeatBaseURL(), monitor.Status.ID, monitor.Status.HeartbeatURL)
	shouldPublish := monitor.Spec.Monitor.Type == urtypes.TypeHeartbeat && heartbeatURL != ""
	if !shouldPublish {
		return r.cleanupHeartbeatURLPublishTarget(ctx, monitor)
	}
	if publish == nil {
		return r.cleanupHeartbeatURLPublishTarget(ctx, monitor)
	}

	targetType := publish.Type
	if targetType == "" {
		targetType = uptimerobotv1.HeartbeatURLPublishTypeSecret
	}
	targetName := publish.Name
	if targetName == "" {
		targetName = fmt.Sprintf("%s-heartbeat-url", monitor.Name)
	}
	targetKey := publish.Key
	if targetKey == "" {
		targetKey = "heartbeatURL"
	}

	targetObjectChanged := currentTargetType != targetType ||
		currentTargetName != targetName
	if targetObjectChanged && currentTargetName != "" {
		if err := r.cleanupHeartbeatURLPublishTargetObject(ctx, monitor, currentTargetType, currentTargetName); err != nil {
			return err
		}
	}

	switch targetType {
	case uptimerobotv1.HeartbeatURLPublishTypeConfigMap:
		if err := r.reconcileHeartbeatURLConfigMap(ctx, monitor, targetName, targetKey, currentTargetKey, heartbeatURL); err != nil {
			return err
		}
	case uptimerobotv1.HeartbeatURLPublishTypeSecret:
		if err := r.reconcileHeartbeatURLSecret(ctx, monitor, targetName, targetKey, currentTargetKey, heartbeatURL); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported heartbeatURLPublish type %q", targetType)
	}

	if monitor.Status.HeartbeatURLPublishTargetType == targetType &&
		monitor.Status.HeartbeatURLPublishTargetName == targetName &&
		monitor.Status.HeartbeatURLPublishTargetKey == targetKey {
		return nil
	}
	monitor.Status.HeartbeatURLPublishTargetType = targetType
	monitor.Status.HeartbeatURLPublishTargetName = targetName
	monitor.Status.HeartbeatURLPublishTargetKey = targetKey
	log.FromContext(ctx).V(1).Info("Published heartbeat URL target",
		"monitor", monitor.Name,
		"targetType", targetType,
		"targetName", targetName,
		"targetKey", targetKey)
	return r.updateMonitorStatus(ctx, monitor)
}

func (r *MonitorReconciler) cleanupHeartbeatURLPublishTarget(ctx context.Context, monitor *uptimerobotv1.Monitor) error {
	targetType := monitor.Status.HeartbeatURLPublishTargetType
	targetName := strings.TrimSpace(monitor.Status.HeartbeatURLPublishTargetName)
	if targetType == "" && targetName == "" && monitor.Status.HeartbeatURLPublishTargetKey == "" {
		return nil
	}

	if targetName != "" {
		if err := r.cleanupHeartbeatURLPublishTargetObject(ctx, monitor, targetType, targetName); err != nil {
			return err
		}
	}

	monitor.Status.HeartbeatURLPublishTargetType = ""
	monitor.Status.HeartbeatURLPublishTargetName = ""
	monitor.Status.HeartbeatURLPublishTargetKey = ""
	return r.updateMonitorStatus(ctx, monitor)
}

func (r *MonitorReconciler) cleanupHeartbeatURLPublishTargetObject(ctx context.Context, monitor *uptimerobotv1.Monitor, targetType uptimerobotv1.HeartbeatURLPublishType, targetName string) error {
	switch targetType {
	case uptimerobotv1.HeartbeatURLPublishTypeSecret:
		secret := &corev1.Secret{}
		err := r.Get(ctx, client.ObjectKey{Name: targetName, Namespace: monitor.Namespace}, secret)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if err == nil && metav1.IsControlledBy(secret, monitor) {
			if err := r.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	case uptimerobotv1.HeartbeatURLPublishTypeConfigMap:
		configMap := &corev1.ConfigMap{}
		err := r.Get(ctx, client.ObjectKey{Name: targetName, Namespace: monitor.Namespace}, configMap)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if err == nil && metav1.IsControlledBy(configMap, monitor) {
			if err := r.Delete(ctx, configMap); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	default:
		log.FromContext(ctx).Info("Skipping heartbeat publish cleanup for unknown target type",
			"monitor", monitor.Name,
			"targetType", targetType,
			"targetName", targetName)
	}
	return nil
}

func (r *MonitorReconciler) reconcileHeartbeatURLSecret(ctx context.Context, monitor *uptimerobotv1.Monitor, name, key, previousKey, heartbeatURL string) error {
	logger := log.FromContext(ctx)
	secret := &corev1.Secret{}
	objKey := client.ObjectKey{Name: name, Namespace: monitor.Namespace}
	err := r.Get(ctx, objKey, secret)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get Secret %s/%s for monitor %s: %w", monitor.Namespace, name, monitor.Name, err)
	}

	if apierrors.IsNotFound(err) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: monitor.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				key: []byte(heartbeatURL),
			},
		}
		if err := controllerutil.SetControllerReference(monitor, secret, r.Scheme); err != nil {
			return fmt.Errorf("failed to set owner reference on Secret %s/%s for monitor %s: %w", monitor.Namespace, name, monitor.Name, err)
		}
		if err := r.Create(ctx, secret); err != nil {
			return fmt.Errorf("failed to create Secret %s/%s for monitor %s: %w", monitor.Namespace, name, monitor.Name, err)
		}
		logger.V(1).Info("Created heartbeat URL Secret",
			"monitor", monitor.Name,
			"namespace", monitor.Namespace,
			"secret", name,
			"key", key)
		return nil
	}

	if !metav1.IsControlledBy(secret, monitor) {
		return fmt.Errorf("refusing to publish heartbeat URL to existing Secret %q: object is not managed by Monitor %q", name, monitor.Name)
	}

	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	needsUpdate := false
	if string(secret.Data[key]) != heartbeatURL {
		secret.Data[key] = []byte(heartbeatURL)
		needsUpdate = true
	}
	// If key changed on the same managed Secret, remove the previously managed key.
	if previousKey != "" && previousKey != key {
		if _, exists := secret.Data[previousKey]; exists {
			delete(secret.Data, previousKey)
			needsUpdate = true
		}
	}
	if !needsUpdate {
		return nil
	}
	if err := r.Update(ctx, secret); err != nil {
		return fmt.Errorf("failed to update Secret %s/%s for monitor %s: %w", monitor.Namespace, name, monitor.Name, err)
	}
	logger.V(1).Info("Updated heartbeat URL Secret",
		"monitor", monitor.Name,
		"namespace", monitor.Namespace,
		"secret", name,
		"key", key)
	return nil
}

func (r *MonitorReconciler) reconcileHeartbeatURLConfigMap(ctx context.Context, monitor *uptimerobotv1.Monitor, name, key, previousKey, heartbeatURL string) error {
	logger := log.FromContext(ctx)
	configMap := &corev1.ConfigMap{}
	objKey := client.ObjectKey{Name: name, Namespace: monitor.Namespace}
	err := r.Get(ctx, objKey, configMap)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get ConfigMap %s/%s for monitor %s: %w", monitor.Namespace, name, monitor.Name, err)
	}

	if apierrors.IsNotFound(err) {
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: monitor.Namespace,
			},
			Data: map[string]string{
				key: heartbeatURL,
			},
		}
		if err := controllerutil.SetControllerReference(monitor, configMap, r.Scheme); err != nil {
			return fmt.Errorf("failed to set owner reference on ConfigMap %s/%s for monitor %s: %w", monitor.Namespace, name, monitor.Name, err)
		}
		if err := r.Create(ctx, configMap); err != nil {
			return fmt.Errorf("failed to create ConfigMap %s/%s for monitor %s: %w", monitor.Namespace, name, monitor.Name, err)
		}
		logger.V(1).Info("Created heartbeat URL ConfigMap",
			"monitor", monitor.Name,
			"namespace", monitor.Namespace,
			"configMap", name,
			"key", key)
		return nil
	}

	if !metav1.IsControlledBy(configMap, monitor) {
		return fmt.Errorf("refusing to publish heartbeat URL to existing ConfigMap %q: object is not managed by Monitor %q", name, monitor.Name)
	}

	if configMap.Data == nil {
		configMap.Data = map[string]string{}
	}
	needsUpdate := false
	if configMap.Data[key] != heartbeatURL {
		configMap.Data[key] = heartbeatURL
		needsUpdate = true
	}
	// If key changed on the same managed ConfigMap, remove the previously managed key.
	if previousKey != "" && previousKey != key {
		if _, exists := configMap.Data[previousKey]; exists {
			delete(configMap.Data, previousKey)
			needsUpdate = true
		}
	}
	if !needsUpdate {
		return nil
	}
	if err := r.Update(ctx, configMap); err != nil {
		return fmt.Errorf("failed to update ConfigMap %s/%s for monitor %s: %w", monitor.Namespace, name, monitor.Name, err)
	}
	logger.V(1).Info("Updated heartbeat URL ConfigMap",
		"monitor", monitor.Name,
		"namespace", monitor.Namespace,
		"configMap", name,
		"key", key)
	return nil
}

// updateMonitorStatus writes status, retrying on conflict so a successful create is not
// followed by a second create (409) when the initial status update fails.
func (r *MonitorReconciler) updateMonitorStatus(ctx context.Context, monitor *uptimerobotv1.Monitor) error {
	err := r.Status().Update(ctx, monitor)
	if err == nil {
		return nil
	}
	if !apierrors.IsConflict(err) {
		return err
	}
	// Conflict: re-get latest version and reapply status update so we don't trigger another create.
	latest := &uptimerobotv1.Monitor{}
	if getErr := r.Get(ctx, client.ObjectKeyFromObject(monitor), latest); getErr != nil {
		return getErr
	}
	latest.Status.Ready = monitor.Status.Ready
	latest.Status.ID = monitor.Status.ID
	latest.Status.Type = monitor.Status.Type
	latest.Status.Status = monitor.Status.Status
	latest.Status.HeartbeatURL = monitor.Status.HeartbeatURL
	latest.Status.HeartbeatURLPublishTargetType = monitor.Status.HeartbeatURLPublishTargetType
	latest.Status.HeartbeatURLPublishTargetName = monitor.Status.HeartbeatURLPublishTargetName
	latest.Status.HeartbeatURLPublishTargetKey = monitor.Status.HeartbeatURLPublishTargetKey
	latest.Status.ObservedGeneration = monitor.Status.ObservedGeneration
	latest.Status.Conditions = monitor.Status.Conditions
	return r.Status().Update(ctx, latest)
}

// SetupWithManager sets up the controller with the Manager.
func (r *MonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &uptimerobotv1.Monitor{}, "spec.sourceRef", func(rawObj client.Object) []string {
		monitor := rawObj.(*uptimerobotv1.Monitor)
		if monitor.Spec.SourceRef == nil {
			return nil
		}
		return []string{monitor.Spec.SourceRef.Kind + "/" + monitor.Spec.SourceRef.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&uptimerobotv1.Monitor{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("monitor").
		Complete(r)
}
