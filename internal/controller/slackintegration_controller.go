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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

const slackIntegrationFinalizerName = "uptimerobot.com/slackintegration-finalizer"

// SlackIntegrationReconciler reconciles a SlackIntegration object.
type SlackIntegrationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=uptimerobot.com,resources=slackintegrations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=uptimerobot.com,resources=slackintegrations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=uptimerobot.com,resources=slackintegrations/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile reconciles SlackIntegration resources to UptimeRobot integrations.
func (r *SlackIntegrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	resource := &uptimerobotv1.SlackIntegration{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	account := &uptimerobotv1.Account{}
	if err := GetAccount(ctx, r.Client, account, resource.Spec.Account.Name); err != nil {
		return ctrl.Result{}, err
	}

	apiKey, err := GetApiKey(ctx, r.Client, account)
	if err != nil {
		return ctrl.Result{}, err
	}

	urclient := uptimerobot.NewClient(apiKey)

	if !resource.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(resource, slackIntegrationFinalizerName) {
			if resource.Spec.Prune && resource.Status.ID != "" {
				id, convErr := strconv.Atoi(resource.Status.ID)
				if convErr == nil {
					if err := urclient.DeleteIntegration(ctx, id); err != nil {
						return ctrl.Result{}, err
					}
				}
			}
			controllerutil.RemoveFinalizer(resource, slackIntegrationFinalizerName)
			if err := r.Update(ctx, resource); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	webhookURL, err := r.resolveWebhookURL(ctx, resource)
	if err != nil {
		return ctrl.Result{}, err
	}

	createData := uptimerobot.SlackIntegrationData{
		FriendlyName:           resource.Spec.Integration.FriendlyName,
		EnableNotificationsFor: resource.Spec.Integration.EnableNotificationsFor,
		SSLExpirationReminder:  resource.Spec.Integration.SSLExpirationReminder,
		WebhookURL:             webhookURL,
		CustomValue:            resource.Spec.Integration.CustomValue,
	}

	if !resource.Status.Ready || resource.Status.ID == "" {
		created, err := urclient.CreateSlackIntegration(ctx, createData)
		if err != nil {
			return ctrl.Result{}, err
		}
		resource.Status.Ready = true
		resource.Status.ID = strconv.Itoa(created.ID)
		resource.Status.Type = "Slack"
		if err := r.updateSlackIntegrationStatus(ctx, resource); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		// Ensure the integration still exists; recreate if it was removed externally.
		id, convErr := strconv.Atoi(resource.Status.ID)
		if convErr != nil {
			return ctrl.Result{}, fmt.Errorf("invalid status.id %q: %w", resource.Status.ID, convErr)
		}

		exists := false
		integrations, err := urclient.ListIntegrations(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}
		for _, integration := range integrations {
			if integration.ID == id {
				exists = true
				break
			}
		}

		if !exists {
			created, createErr := urclient.CreateSlackIntegration(ctx, createData)
			if createErr != nil {
				return ctrl.Result{}, createErr
			}
			resource.Status.Ready = true
			resource.Status.ID = strconv.Itoa(created.ID)
			resource.Status.Type = "Slack"
			if err := r.updateSlackIntegrationStatus(ctx, resource); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if !controllerutil.ContainsFinalizer(resource, slackIntegrationFinalizerName) {
		controllerutil.AddFinalizer(resource, slackIntegrationFinalizerName)
		if err := r.Update(ctx, resource); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: resource.Spec.SyncInterval.Duration}, nil
}

func (r *SlackIntegrationReconciler) resolveWebhookURL(ctx context.Context, resource *uptimerobotv1.SlackIntegration) (string, error) {
	if resource.Spec.Integration.WebhookURL != "" {
		return resource.Spec.Integration.WebhookURL, nil
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: resource.Namespace,
		Name:      resource.Spec.Integration.SecretName,
	}, secret); err != nil {
		return "", err
	}

	key := resource.Spec.Integration.WebhookURLKey
	if key == "" {
		key = "webhookURL"
	}
	value, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrSecretMissingKey, key)
	}
	return string(value), nil
}

func (r *SlackIntegrationReconciler) updateSlackIntegrationStatus(ctx context.Context, resource *uptimerobotv1.SlackIntegration) error {
	err := r.Status().Update(ctx, resource)
	if err == nil {
		return nil
	}
	if !apierrors.IsConflict(err) {
		return err
	}

	latest := &uptimerobotv1.SlackIntegration{}
	if getErr := r.Get(ctx, client.ObjectKeyFromObject(resource), latest); getErr != nil {
		return getErr
	}
	latest.Status.Ready = resource.Status.Ready
	latest.Status.ID = resource.Status.ID
	latest.Status.Type = resource.Status.Type
	return r.Status().Update(ctx, latest)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SlackIntegrationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&uptimerobotv1.SlackIntegration{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("slackintegration").
		Complete(r)
}
