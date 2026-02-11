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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Conditions", func() {
	Context("SetCondition", func() {
		It("should add a new condition", func() {
			conditions := []metav1.Condition{}
			SetCondition(&conditions, TypeReady, metav1.ConditionTrue, ReasonReconcileSuccess, "All good", 1)

			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(TypeReady))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(conditions[0].Reason).To(Equal(ReasonReconcileSuccess))
			Expect(conditions[0].Message).To(Equal("All good"))
			Expect(conditions[0].ObservedGeneration).To(Equal(int64(1)))
		})

		It("should update an existing condition", func() {
			conditions := []metav1.Condition{
				{
					Type:               TypeReady,
					Status:             metav1.ConditionFalse,
					LastTransitionTime: metav1.Now(),
					Reason:             ReasonReconcileError,
					Message:            "Error occurred",
					ObservedGeneration: 1,
				},
			}

			// Update the condition
			SetCondition(&conditions, TypeReady, metav1.ConditionTrue, ReasonReconcileSuccess, "All good now", 2)

			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(TypeReady))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(conditions[0].Reason).To(Equal(ReasonReconcileSuccess))
			Expect(conditions[0].Message).To(Equal("All good now"))
			Expect(conditions[0].ObservedGeneration).To(Equal(int64(2)))
		})

		It("should preserve other conditions when updating one", func() {
			conditions := []metav1.Condition{
				{
					Type:               TypeReady,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             ReasonReconcileSuccess,
					Message:            "Ready",
					ObservedGeneration: 1,
				},
				{
					Type:               TypeSynced,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             ReasonSyncSuccess,
					Message:            "Synced",
					ObservedGeneration: 1,
				},
			}

			// Update only the Error condition
			SetCondition(&conditions, TypeError, metav1.ConditionTrue, ReasonAPIError, "API error", 2)

			Expect(conditions).To(HaveLen(3))
			Expect(conditions[0].Type).To(Equal(TypeReady))
			Expect(conditions[1].Type).To(Equal(TypeSynced))
			Expect(conditions[2].Type).To(Equal(TypeError))
			Expect(conditions[2].Status).To(Equal(metav1.ConditionTrue))
			Expect(conditions[2].Reason).To(Equal(ReasonAPIError))
			Expect(conditions[2].Message).To(Equal("API error"))
		})
	})

	Context("SetReadyCondition", func() {
		It("should set Ready condition to True", func() {
			conditions := []metav1.Condition{}
			SetReadyCondition(&conditions, true, ReasonReconcileSuccess, "Success", 1)

			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(TypeReady))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))
		})

		It("should set Ready condition to False", func() {
			conditions := []metav1.Condition{}
			SetReadyCondition(&conditions, false, ReasonReconcileError, "Error", 1)

			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(TypeReady))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("SetSyncedCondition", func() {
		It("should set Synced condition to True", func() {
			conditions := []metav1.Condition{}
			SetSyncedCondition(&conditions, true, ReasonSyncSuccess, "Synced", 1)

			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(TypeSynced))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))
		})

		It("should set Synced condition to False", func() {
			conditions := []metav1.Condition{}
			SetSyncedCondition(&conditions, false, ReasonSyncError, "Sync failed", 1)

			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(TypeSynced))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("SetErrorCondition", func() {
		It("should set Error condition to True", func() {
			conditions := []metav1.Condition{}
			SetErrorCondition(&conditions, true, ReasonAPIError, "API error", 1)

			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(TypeError))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))
		})

		It("should set Error condition to False", func() {
			conditions := []metav1.Condition{}
			SetErrorCondition(&conditions, false, ReasonReconcileSuccess, "", 1)

			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal(TypeError))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionFalse))
		})
	})
})
