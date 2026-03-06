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
	"time"

	"github.com/joelp172/uptime-robot-operator/internal/metrics"
	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

// ContactReconciler reconciles a Contact object
type ContactReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=uptimerobot.com,resources=contacts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=uptimerobot.com,resources=contacts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=uptimerobot.com,resources=contacts/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *ContactReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.ReconciliationDuration.WithLabelValues("contact").Observe(duration)
	}()

	_ = log.FromContext(ctx)

	contact := &uptimerobotv1.Contact{}
	if err := r.Get(ctx, req.NamespacedName, contact); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	contact.Status.ObservedGeneration = contact.Generation

	account := &uptimerobotv1.Account{}
	if err := GetAccount(ctx, r.Client, account, contact.Spec.Account.Name); err != nil {
		metrics.ReconciliationErrorsTotal.WithLabelValues("contact", "account_not_found").Inc()
		contact.Status.Ready = false
		msg := "Failed to get account: " + err.Error()
		// Don't set Synced here since we haven't attempted sync with UptimeRobot yet
		SetReadyCondition(&contact.Status.Conditions, false, ReasonReconcileError, msg, contact.Generation)
		SetErrorCondition(&contact.Status.Conditions, true, ReasonReconcileError, msg, contact.Generation)
		if r.Recorder != nil {
			r.Recorder.Event(contact, "Warning", "DependencyNotReady", msg)
		}
		if updateErr := r.Status().Update(ctx, contact); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	apiKey, err := GetApiKey(ctx, r.Client, account)
	if err != nil {
		metrics.ReconciliationErrorsTotal.WithLabelValues("contact", "secret_not_found").Inc()
		contact.Status.Ready = false
		msg := "Failed to get API key: " + err.Error()
		// Don't set Synced here since we haven't attempted sync with UptimeRobot yet
		SetReadyCondition(&contact.Status.Conditions, false, ReasonSecretNotFound, msg, contact.Generation)
		SetErrorCondition(&contact.Status.Conditions, true, ReasonSecretNotFound, msg, contact.Generation)
		if r.Recorder != nil {
			r.Recorder.Event(contact, "Warning", "SecretNotFound", msg)
		}
		if updateErr := r.Status().Update(ctx, contact); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	urclient := uptimerobot.NewClient(apiKey)

	if contact.Status.ID == "" {
		var id string
		validated := false

		// If ID is specified directly, use it; otherwise look up by name
		if contact.Spec.Contact.ID != "" {
			id = contact.Spec.Contact.ID
		} else if contact.Spec.Contact.Name != "" {
			var err error
			id, err = urclient.FindContactID(ctx, contact.Spec.Contact.Name)
			if err != nil {
				metrics.ReconciliationErrorsTotal.WithLabelValues("contact", "api_error").Inc()
				contact.Status.Ready = false
				msg := "Failed to find contact: " + err.Error()
				SetReadyCondition(&contact.Status.Conditions, false, ReasonAPIError, msg, contact.Generation)
				SetSyncedCondition(&contact.Status.Conditions, false, ReasonSyncError, msg, contact.Generation)
				SetErrorCondition(&contact.Status.Conditions, true, ReasonAPIError, msg, contact.Generation)
				if r.Recorder != nil {
					r.Recorder.Event(contact, "Warning", "SyncFailed", msg)
				}

				// Track retry count and apply exponential backoff for transient errors
				retryCount := GetRetryCount(contact.Annotations)
				if IsTransientError(err) {
					contact.Annotations = IncrementRetryCount(contact.Annotations)
					// Save status before r.Update, which refreshes the object from the server
					// (clearing any in-memory status changes not yet persisted to the status subresource).
					savedStatus := contact.Status
					if updateErr := r.Update(ctx, contact); updateErr != nil {
						return ctrl.Result{}, updateErr
					}
					contact.Status = savedStatus
				}

				if updateErr := r.Status().Update(ctx, contact); updateErr != nil {
					return ctrl.Result{}, updateErr
				}
				return HandleReconcileError(err, retryCount)
			}
			validated = true
		} else {
			metrics.ReconciliationErrorsTotal.WithLabelValues("contact", "unknown").Inc()
			err := errors.New("contact must specify either id or name")
			contact.Status.Ready = false
			// Don't set Synced here since this is a validation error before sync attempt
			SetReadyCondition(&contact.Status.Conditions, false, ReasonReconcileError, err.Error(), contact.Generation)
			SetErrorCondition(&contact.Status.Conditions, true, ReasonReconcileError, err.Error(), contact.Generation)
			if updateErr := r.Status().Update(ctx, contact); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, err
		}

		contact.Status.Ready = true
		contact.Status.ID = id
		SetReadyCondition(&contact.Status.Conditions, true, ReasonReconcileSuccess, "Contact reconciled successfully", contact.Generation)
		if validated {
			SetSyncedCondition(&contact.Status.Conditions, true, ReasonSyncSuccess, "Successfully validated contact reference", contact.Generation)
			if r.Recorder != nil {
				r.Recorder.Event(contact, "Normal", "Synced", "Contact reconciled successfully")
			}
		} else {
			SetCondition(&contact.Status.Conditions, TypeSynced, metav1.ConditionUnknown, ReasonSyncSkipped, "Skipped contact validation; using spec.contact.id", contact.Generation)
		}
		SetErrorCondition(&contact.Status.Conditions, false, ReasonReconcileSuccess, "", contact.Generation)
		if err := r.Status().Update(ctx, contact); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		contact.Status.Ready = true
		SetReadyCondition(&contact.Status.Conditions, true, ReasonReconcileSuccess, "Contact reconciled successfully", contact.Generation)
		SetCondition(&contact.Status.Conditions, TypeSynced, metav1.ConditionUnknown, ReasonSyncSkipped, "Skipped contact validation; using existing status.id", contact.Generation)
		SetErrorCondition(&contact.Status.Conditions, false, ReasonReconcileSuccess, "", contact.Generation)
		if err := r.Status().Update(ctx, contact); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ContactReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &uptimerobotv1.Contact{}, "spec.isDefault", func(rawObj client.Object) []string {
		contact := rawObj.(*uptimerobotv1.Contact)
		if !contact.Spec.IsDefault {
			return nil
		}
		return []string{"true"}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&uptimerobotv1.Contact{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("contact").
		Complete(r)
}

var (
	ErrNoDefaultContact       = errors.New("no default contact")
	ErrMultipleDefaultContact = errors.New("more than 1 default contact found")
)

func GetContact(ctx context.Context, c client.Client, contact *uptimerobotv1.Contact, name string) error {
	if name != "" {
		return c.Get(ctx, client.ObjectKey{Name: name}, contact)
	}

	list := &uptimerobotv1.ContactList{}
	err := c.List(ctx, list, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.isDefault", "true"),
	})
	if err != nil {
		return err
	}
	if len(list.Items) == 0 {
		return ErrNoDefaultContact
	}
	if len(list.Items) > 1 {
		return ErrMultipleDefaultContact
	}

	*contact = list.Items[0]
	return nil
}
