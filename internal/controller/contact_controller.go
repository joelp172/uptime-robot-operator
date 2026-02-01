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

	"github.com/joelp172/uptime-robot-operator/internal/uptimerobot"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
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
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=uptimerobot.com,resources=contacts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=uptimerobot.com,resources=contacts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=uptimerobot.com,resources=contacts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *ContactReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	contact := &uptimerobotv1.Contact{}
	if err := r.Client.Get(ctx, req.NamespacedName, contact); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	account := &uptimerobotv1.Account{}
	if err := GetAccount(ctx, r.Client, account, contact.Spec.Account.Name); err != nil {
		return ctrl.Result{}, err
	}

	apiKey, err := GetApiKey(ctx, r.Client, account)
	if err != nil {
		return ctrl.Result{}, err
	}

	urclient := uptimerobot.NewClient(apiKey)

	if contact.Status.ID == "" {
		id, err := urclient.FindContactID(ctx, contact.Spec.Contact.Name)
		if err != nil {
			return ctrl.Result{}, err
		}

		contact.Status.Ready = true
		contact.Status.ID = id
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
