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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SlackIntegrationSpec defines the desired state of SlackIntegration.
type SlackIntegrationSpec struct {
	// SyncInterval defines how often the operator reconciles with the UptimeRobot API.
	//+kubebuilder:default:="24h"
	SyncInterval *metav1.Duration `json:"syncInterval,omitempty"`

	// Prune enables garbage collection.
	//+kubebuilder:default:=true
	Prune bool `json:"prune"`

	// Account references this object's Account. If not specified, the default will be used.
	Account corev1.LocalObjectReference `json:"account,omitempty"`

	// Integration configures the Slack integration settings.
	Integration SlackIntegrationValues `json:"integration"`
}

// SlackIntegrationStatus defines the observed state of SlackIntegration.
type SlackIntegrationStatus struct {
	Ready bool `json:"ready"`
	// ID is the UptimeRobot integration ID.
	ID string `json:"id,omitempty"`
	// Type is the UptimeRobot integration type (e.g. Slack).
	Type string `json:"type,omitempty"`
}

// SlackIntegrationValues defines the desired Slack integration settings.
// +kubebuilder:validation:XValidation:rule="(has(self.webhookURL) && self.webhookURL != \"\") || (has(self.secretName) && self.secretName != \"\")",message="either webhookURL or secretName must be specified"
// +kubebuilder:validation:XValidation:rule="!(has(self.webhookURL) && self.webhookURL != \"\" && has(self.secretName) && self.secretName != \"\")",message="specify only one of webhookURL or secretName"
type SlackIntegrationValues struct {
	// FriendlyName is the display name shown in UptimeRobot.
	//+kubebuilder:validation:MaxLength=60
	FriendlyName string `json:"friendlyName,omitempty"`

	// EnableNotificationsFor controls which events trigger notifications.
	//+kubebuilder:validation:Enum=UpAndDown;Down;Up;None
	//+kubebuilder:default:=UpAndDown
	EnableNotificationsFor string `json:"enableNotificationsFor,omitempty"`

	// SSLExpirationReminder enables notifications for SSL/Domain expiry checks.
	SSLExpirationReminder bool `json:"sslExpirationReminder,omitempty"`

	// WebhookURL is the Slack webhook URL.
	//+kubebuilder:validation:MaxLength=1500
	WebhookURL string `json:"webhookURL,omitempty"`

	// SecretName is the secret containing the webhook URL.
	// If set, WebhookURL must be omitted.
	SecretName string `json:"secretName,omitempty"`

	// WebhookURLKey is the key in the secret containing the webhook URL.
	//+kubebuilder:default:=webhookURL
	WebhookURLKey string `json:"webhookURLKey,omitempty"`

	// CustomValue is optional text appended to each notification payload.
	//+kubebuilder:validation:MaxLength=5000
	CustomValue string `json:"customValue,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
//+kubebuilder:printcolumn:name="Type",type="string",JSONPath=".status.type"
//+kubebuilder:printcolumn:name="ID",type="string",JSONPath=".status.id"
//+kubebuilder:printcolumn:name="Friendly Name",type="string",JSONPath=".spec.integration.friendlyName"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SlackIntegration is the Schema for the slackintegrations API.
type SlackIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SlackIntegrationSpec   `json:"spec,omitempty"`
	Status SlackIntegrationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SlackIntegrationList contains a list of SlackIntegration.
type SlackIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SlackIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SlackIntegration{}, &SlackIntegrationList{})
}
