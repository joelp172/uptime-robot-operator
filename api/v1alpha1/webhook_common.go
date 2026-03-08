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

package v1alpha1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// validateAccountRef checks that the referenced Account exists. When accountName
// is empty the function verifies that exactly one default Account is present.
func validateAccountRef(ctx context.Context, c client.Reader, accountName string, fldPath *field.Path, kind string) *field.Error {
	if accountName != "" {
		account := &Account{}
		if err := c.Get(ctx, client.ObjectKey{Name: accountName}, account); err != nil {
			if apierrors.IsNotFound(err) {
				return field.Invalid(fldPath, accountName, fmt.Sprintf("Account %q not found", accountName))
			}
			return field.InternalError(fldPath, fmt.Errorf("looking up Account %q: %w", accountName, err))
		}
		return nil
	}

	// No name specified — fall back to a default account.
	list := &AccountList{}
	if err := c.List(ctx, list); err != nil {
		return field.InternalError(fldPath, fmt.Errorf("listing accounts: %w", err))
	}

	defaultCount := 0
	for i := range list.Items {
		if list.Items[i].Spec.IsDefault {
			defaultCount++
		}
	}

	if defaultCount == 1 {
		return nil
	}
	if defaultCount == 0 {
		return field.Required(fldPath,
			fmt.Sprintf("no account name specified and no default Account exists for %s", kind))
	}
	return field.Invalid(fldPath, accountName,
		fmt.Sprintf("no account name specified and multiple default Accounts exist for %s", kind))
}

// invalidErr wraps a field.ErrorList in an apierrors.StatusError for the given kind and name.
func invalidErr(kind, name string, errs field.ErrorList) error {
	return apierrors.NewInvalid(
		schema.GroupKind{Group: GroupVersion.Group, Kind: kind},
		name,
		errs,
	)
}
