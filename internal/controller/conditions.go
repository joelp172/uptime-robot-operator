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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Standard condition types for all CRDs
const (
	// TypeReady indicates that the resource is ready for use
	TypeReady = "Ready"
	// TypeSynced indicates that the last sync to UptimeRobot succeeded
	TypeSynced = "Synced"
	// TypeError indicates that the last reconciliation encountered an error
	TypeError = "Error"
	// TypeDeleting indicates that the resource is being deleted (moved from finalizer.go to avoid duplication)
	TypeDeleting = "Deleting"
)

// Standard condition reasons
const (
	// ReasonReconcileSuccess indicates successful reconciliation
	ReasonReconcileSuccess = "ReconcileSuccess"
	// ReasonReconcileError indicates reconciliation error
	ReasonReconcileError = "ReconcileError"
	// ReasonSyncSuccess indicates successful sync to UptimeRobot
	ReasonSyncSuccess = "SyncSuccess"
	// ReasonSyncError indicates sync error to UptimeRobot
	ReasonSyncError = "SyncError"
	// ReasonSyncSkipped indicates sync was intentionally skipped
	ReasonSyncSkipped = "SyncSkipped"
	// ReasonAPIError indicates UptimeRobot API error
	ReasonAPIError = "APIError"
	// ReasonSecretNotFound indicates secret not found
	ReasonSecretNotFound = "SecretNotFound"
)

// SetCondition sets or updates a condition in the conditions list
func SetCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string, observedGeneration int64) {
	if conditions == nil {
		return
	}

	now := metav1.Now()

	// Find existing condition
	for i, condition := range *conditions {
		if condition.Type == conditionType {
			// Update existing condition
			if condition.Status != status || condition.Reason != reason || condition.Message != message {
				(*conditions)[i].Status = status
				(*conditions)[i].Reason = reason
				(*conditions)[i].Message = message
				(*conditions)[i].LastTransitionTime = now
			}
			// Always refresh observedGeneration so consumers know latest generation was evaluated.
			(*conditions)[i].ObservedGeneration = observedGeneration
			return
		}
	}

	// Add new condition
	*conditions = append(*conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGeneration,
	})
}

// SetReadyCondition sets the Ready condition
func SetReadyCondition(conditions *[]metav1.Condition, ready bool, reason, message string, observedGeneration int64) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}
	SetCondition(conditions, TypeReady, status, reason, message, observedGeneration)
}

// SetSyncedCondition sets the Synced condition
func SetSyncedCondition(conditions *[]metav1.Condition, synced bool, reason, message string, observedGeneration int64) {
	status := metav1.ConditionTrue
	if !synced {
		status = metav1.ConditionFalse
	}
	SetCondition(conditions, TypeSynced, status, reason, message, observedGeneration)
}

// SetErrorCondition sets the Error condition
func SetErrorCondition(conditions *[]metav1.Condition, hasError bool, reason, message string, observedGeneration int64) {
	status := metav1.ConditionTrue
	if !hasError {
		status = metav1.ConditionFalse
	}
	SetCondition(conditions, TypeError, status, reason, message, observedGeneration)
}
