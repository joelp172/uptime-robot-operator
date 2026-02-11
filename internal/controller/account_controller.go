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

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

var ClusterResourceNamespace = "uptime-robot-system"

// AccountReconciler reconciles a Account object
type AccountReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

var ErrKeyNotFound = errors.New("secret key not found")

//+kubebuilder:rbac:groups=uptimerobot.com,resources=accounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=uptimerobot.com,resources=accounts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=uptimerobot.com,resources=accounts/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *AccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	account := &uptimerobotv1.Account{}
	if err := r.Get(ctx, req.NamespacedName, account); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	apiKey, err := GetApiKey(ctx, r.Client, account)
	if err != nil {
		account.Status.Ready = false
		// Don't set Synced here since we haven't attempted sync with UptimeRobot yet
		SetReadyCondition(&account.Status.Conditions, false, ReasonSecretNotFound, fmt.Sprintf("Failed to get API key: %v", err), account.Generation)
		SetErrorCondition(&account.Status.Conditions, true, ReasonSecretNotFound, fmt.Sprintf("Failed to get API key: %v", err), account.Generation)
		if updateErr := r.Status().Update(ctx, account); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	urclient := uptimerobot.NewClient(apiKey)
	email, err := urclient.GetAccountDetails(ctx)
	if err != nil {
		account.Status.Ready = false
		SetReadyCondition(&account.Status.Conditions, false, ReasonAPIError, fmt.Sprintf("Failed to get account details: %v", err), account.Generation)
		SetSyncedCondition(&account.Status.Conditions, false, ReasonSyncError, fmt.Sprintf("Failed to sync with UptimeRobot: %v", err), account.Generation)
		SetErrorCondition(&account.Status.Conditions, true, ReasonAPIError, fmt.Sprintf("Failed to get account details: %v", err), account.Generation)
		if updateErr := r.Status().Update(ctx, account); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	// Fetch alert contacts
	contacts, err := urclient.GetAlertContacts(ctx)
	if err != nil {
		log.FromContext(ctx).Error(err, "failed to fetch alert contacts")
		// Don't fail the reconciliation if we can't get contacts
	}

	// Convert to status format
	alertContacts := make([]uptimerobotv1.AlertContactInfo, 0, len(contacts))
	for _, c := range contacts {
		info := uptimerobotv1.AlertContactInfo{
			ID:    fmt.Sprintf("%d", c.ID),
			Type:  c.Type,
			Value: c.Value,
		}
		if c.FriendlyName != nil {
			info.FriendlyName = *c.FriendlyName
		}
		alertContacts = append(alertContacts, info)
	}

	account.Status.Ready = true
	account.Status.Email = email
	account.Status.AlertContacts = alertContacts
	account.Status.ObservedGeneration = account.Generation
	SetReadyCondition(&account.Status.Conditions, true, ReasonReconcileSuccess, "Account reconciled successfully", account.Generation)
	SetSyncedCondition(&account.Status.Conditions, true, ReasonSyncSuccess, "Successfully synced with UptimeRobot", account.Generation)
	SetErrorCondition(&account.Status.Conditions, false, ReasonReconcileSuccess, "", account.Generation)
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
		Named("account").
		Complete(r)
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

	return string(apiKey), nil
}
