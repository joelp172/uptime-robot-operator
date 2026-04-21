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

import corev1 "k8s.io/api/core/v1"

// IntegrationAccountReferencer is satisfied by any integration CRD that
// references a parent Account. It lets the Account reconciler enqueue an
// Account when one of its integration children changes without each
// integration needing a bespoke mapper.
//
// To bridge a new integration CRD:
//  1. Add a GetAccountRef() corev1.LocalObjectReference method on the CRD type.
//  2. Add a .Watches(&NewIntegration{}, handler.EnqueueRequestsFromMapFunc(r.mapIntegrationToAccount))
//     line in AccountReconciler.SetupWithManager.
//  3. Append the integration's UptimeRobot API type string to
//     uptimerobot.BridgedIntegrationTypes so it is surfaced in Account status
//     and resolvable via Contact friendly-name lookup.
//
// The interface lives here (not in the api package) because controller-gen's
// deepcopy generator cannot process interface types in api/v1alpha1.
type IntegrationAccountReferencer interface {
	GetAccountRef() corev1.LocalObjectReference
}
