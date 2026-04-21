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
	"strconv"
	"strings"
	"time"

	"github.com/joelp172/uptime-robot-operator/internal/metrics"
	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

var ClusterResourceNamespace = "uptime-robot-system"

// AccountReconciler reconciles a Account object
type AccountReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

var (
	ErrKeyNotFound = errors.New("secret key not found")
	ErrEmptyKey    = errors.New("secret key value is empty")
)

//+kubebuilder:rbac:groups=uptimerobot.com,resources=accounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=uptimerobot.com,resources=accounts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=uptimerobot.com,resources=accounts/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *AccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.ReconciliationDuration.WithLabelValues("account").Observe(duration)
	}()

	_ = log.FromContext(ctx)

	account := &uptimerobotv1.Account{}
	if err := r.Get(ctx, req.NamespacedName, account); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	account.Status.ObservedGeneration = account.Generation

	apiKey, err := GetApiKey(ctx, r.Client, account)
	if err != nil {
		metrics.ReconciliationErrorsTotal.WithLabelValues("account", "secret_not_found").Inc()
		account.Status.Ready = false
		msg := fmt.Sprintf("Failed to get API key: %v", err)
		// Don't set Synced here since we haven't attempted sync with UptimeRobot yet
		SetReadyCondition(&account.Status.Conditions, false, ReasonSecretNotFound, msg, account.Generation)
		SetErrorCondition(&account.Status.Conditions, true, ReasonSecretNotFound, msg, account.Generation)
		if r.Recorder != nil {
			r.Recorder.Event(account, "Warning", "SecretNotFound", msg)
		}
		if updateErr := r.Status().Update(ctx, account); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	urclient := uptimerobot.NewClient(apiKey)
	email, err := urclient.GetAccountDetails(ctx)
	if err != nil {
		metrics.ReconciliationErrorsTotal.WithLabelValues("account", "api_error").Inc()
		account.Status.Ready = false
		msg := fmt.Sprintf("Failed to get account details: %v", err)
		SetReadyCondition(&account.Status.Conditions, false, ReasonAPIError, msg, account.Generation)
		SetSyncedCondition(&account.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to sync with UptimeRobot: %v", err), account.Generation)
		SetErrorCondition(&account.Status.Conditions, true, ReasonAPIError, msg, account.Generation)
		if r.Recorder != nil {
			r.Recorder.Event(account, "Warning", "SyncFailed", msg)
		}

		// Track retry count and apply exponential backoff for transient errors
		retryCount := GetRetryCount(account.Annotations)
		if IsTransientError(err) {
			account.Annotations = IncrementRetryCount(account.Annotations)
			// Save status before r.Update, which replaces the in-memory object with the
			// server's version (where status changes haven't been persisted yet via
			// Status().Update()). Without this, the status conditions set above would be lost.
			savedStatus := account.Status
			if updateErr := r.Update(ctx, account); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			account.Status = savedStatus
		}

		if updateErr := r.Status().Update(ctx, account); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return HandleReconcileError(err, retryCount)
	}

	// Fetch alert contacts. A failure here is non-fatal: we still surface the
	// account itself, but we track the degraded state below so users see why
	// their contact discovery surface is missing entries.
	var degradedReasons []string
	contacts, err := urclient.GetAlertContacts(ctx)
	if err != nil {
		log.FromContext(ctx).Error(err, "failed to fetch alert contacts",
			"account", account.Name, "namespace", account.Namespace)
		degradedReasons = append(degradedReasons, fmt.Sprintf("alert contacts: %v", err))
	}
	integrations, err := urclient.ListIntegrations(ctx)
	if err != nil {
		log.FromContext(ctx).Error(err, "failed to fetch integrations",
			"account", account.Name, "namespace", account.Namespace)
		degradedReasons = append(degradedReasons, fmt.Sprintf("integrations: %v", err))
	}

	// Convert to status format. Alert-contact IDs and integration IDs come
	// from independent upstream namespaces, so dedupe on (type, id) rather
	// than id alone to avoid silently dropping a Slack integration that
	// happens to share a numeric ID with an unrelated alert contact.
	alertContacts := make([]uptimerobotv1.AlertContactInfo, 0, len(contacts)+len(integrations))
	seen := make(map[string]struct{}, len(contacts)+len(integrations))
	dedupeKey := func(contactType, id string) string { return contactType + ":" + id }
	for _, c := range contacts {
		info := uptimerobotv1.AlertContactInfo{
			ID:    strconv.Itoa(c.ID),
			Type:  c.Type,
			Value: c.Value,
		}
		if c.FriendlyName != nil {
			info.FriendlyName = *c.FriendlyName
		}
		alertContacts = append(alertContacts, info)
		seen[dedupeKey(info.Type, info.ID)] = struct{}{}
	}
	for _, integration := range integrations {
		if integration.Type == nil || !uptimerobot.IsBridgedIntegrationType(*integration.Type) {
			continue
		}

		id := strconv.Itoa(integration.ID)
		if _, exists := seen[dedupeKey(*integration.Type, id)]; exists {
			continue
		}

		info := uptimerobotv1.AlertContactInfo{
			ID:    id,
			Type:  *integration.Type,
			Value: integration.Value,
		}
		if integration.FriendlyName != nil {
			info.FriendlyName = *integration.FriendlyName
		}
		alertContacts = append(alertContacts, info)
		seen[dedupeKey(info.Type, info.ID)] = struct{}{}
	}

	account.Status.Ready = true
	account.Status.Email = email
	account.Status.AlertContacts = alertContacts
	if len(degradedReasons) > 0 {
		msg := fmt.Sprintf("Reconciled with partial contact discovery: %s", strings.Join(degradedReasons, "; "))
		// Ready reflects the degraded reconcile; Synced stays "success" to
		// match the semantics used by other controllers (Synced=true means
		// the last UptimeRobot write/sync completed without error — partial
		// *reads* during discovery don't invalidate that).
		SetReadyCondition(&account.Status.Conditions, true, ReasonReconcileDegraded, msg, account.Generation)
		SetSyncedCondition(&account.Status.Conditions, true, ReasonSyncSuccess, "Successfully synced with UptimeRobot", account.Generation)
		SetErrorCondition(&account.Status.Conditions, false, ReasonReconcileDegraded, "", account.Generation)
		if r.Recorder != nil {
			r.Recorder.Event(account, "Warning", "ReconcileDegraded", msg)
		}
	} else {
		SetReadyCondition(&account.Status.Conditions, true, ReasonReconcileSuccess, "Account reconciled successfully", account.Generation)
		SetSyncedCondition(&account.Status.Conditions, true, ReasonSyncSuccess, "Successfully synced with UptimeRobot", account.Generation)
		SetErrorCondition(&account.Status.Conditions, false, ReasonReconcileSuccess, "", account.Generation)
		if r.Recorder != nil {
			r.Recorder.Event(account, "Normal", "Synced", "Account reconciled successfully")
		}
	}
	if err := r.Status().Update(ctx, account); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &uptimerobotv1.Account{}, "spec.isDefault", func(rawObj client.Object) []string {
		account := rawObj.(*uptimerobotv1.Account)
		if !account.Spec.IsDefault {
			return nil
		}
		return []string{"true"}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&uptimerobotv1.Account{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.mapSecretToAccounts)).
		// Integration CRDs that bridge into Account.status.alertContacts are
		// watched here so Account discoverability refreshes as soon as an
		// integration is created, updated, or deleted. Add a new line for
		// each new bridged integration CRD.
		Watches(&uptimerobotv1.SlackIntegration{}, handler.EnqueueRequestsFromMapFunc(r.mapIntegrationToAccount)).
		Named("account").
		Complete(r)
}

// mapIntegrationToAccount enqueues the Account referenced by an integration
// CRD's spec.account. Any object implementing IntegrationAccountReferencer
// works — no Slack-specific logic here. When the integration omits
// spec.account.name, the default Account is enqueued (same resolution rule
// GetAccount uses at reconcile time).
func (r *AccountReconciler) mapIntegrationToAccount(ctx context.Context, obj client.Object) []reconcile.Request {
	ref, ok := obj.(IntegrationAccountReferencer)
	if !ok {
		return nil
	}
	if name := ref.GetAccountRef().Name; name != "" {
		return []reconcile.Request{{NamespacedName: client.ObjectKey{Name: name}}}
	}

	defaults := &uptimerobotv1.AccountList{}
	if err := r.List(ctx, defaults, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.isDefault", "true"),
	}); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(defaults.Items))
	for _, account := range defaults.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Name: account.Name}})
	}
	return requests
}

func (r *AccountReconciler) mapSecretToAccounts(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj.GetNamespace() != ClusterResourceNamespace {
		return nil
	}

	accounts := &uptimerobotv1.AccountList{}
	if err := r.List(ctx, accounts); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(accounts.Items))
	for _, account := range accounts.Items {
		if account.Spec.ApiKeySecretRef.Name == obj.GetName() {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKey{Name: account.Name},
			})
		}
	}
	return requests
}

var (
	ErrNoDefaultAccount       = errors.New("no default account")
	ErrMultipleDefaultAccount = errors.New("more than 1 default account found")
)

func GetAccount(ctx context.Context, c client.Client, account *uptimerobotv1.Account, name string) error {
	if name != "" {
		return c.Get(ctx, client.ObjectKey{Name: name}, account)
	}

	list := &uptimerobotv1.AccountList{}
	err := c.List(ctx, list, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.isDefault", "true"),
	})
	if err != nil {
		return err
	}
	if len(list.Items) == 0 {
		return ErrNoDefaultAccount
	}
	if len(list.Items) > 1 {
		return ErrMultipleDefaultAccount
	}

	*account = list.Items[0]
	return nil
}

func GetApiKey(ctx context.Context, c client.Client, account *uptimerobotv1.Account) (string, error) {
	secret := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKey{
		Namespace: ClusterResourceNamespace,
		Name:      account.Spec.ApiKeySecretRef.Name,
	}, secret)
	if err != nil {
		return "", err
	}

	apiKey, ok := secret.Data[account.Spec.ApiKeySecretRef.Key]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrKeyNotFound, account.Spec.ApiKeySecretRef.Key)
	}

	trimmed := strings.TrimSpace(string(apiKey))
	if trimmed == "" {
		return "", fmt.Errorf("%w: %s", ErrEmptyKey, account.Spec.ApiKeySecretRef.Key)
	}

	return trimmed, nil
}
