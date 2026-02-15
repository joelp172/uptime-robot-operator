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

package urtypes

import (
	"encoding/json"
	"testing"
)

func TestAssertionOperator_ToAPIString(t *testing.T) {
	tests := []struct {
		op       AssertionOperator
		expected string
	}{
		{AssertionEquals, "equals"},
		{AssertionNotEquals, "not_equals"},
		{AssertionContains, "contains"},
		{AssertionNotContains, "not_contains"},
		{AssertionGreaterThan, "greater_than"},
		{AssertionLessThan, "less_than"},
		{AssertionIsNull, "is_null"},
		{AssertionIsNotNull, "is_not_null"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.op.ToAPIString()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestAssertionOperatorFromAPIString(t *testing.T) {
	tests := []struct {
		input    string
		expected AssertionOperator
	}{
		{"equals", AssertionEquals},
		{"not_equals", AssertionNotEquals},
		{"contains", AssertionContains},
		{"not_contains", AssertionNotContains},
		{"greater_than", AssertionGreaterThan},
		{"less_than", AssertionLessThan},
		{"is_null", AssertionIsNull},
		{"is_not_null", AssertionIsNotNull},
		{"invalid", AssertionEquals}, // Default case
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := AssertionOperatorFromAPIString(tt.input)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestAssertionOperatorString_AcceptsAPIStrings(t *testing.T) {
	tests := []struct {
		input    string
		expected AssertionOperator
	}{
		{"not_equals", AssertionNotEquals},
		{"not_contains", AssertionNotContains},
		{"greater_than", AssertionGreaterThan},
		{"less_than", AssertionLessThan},
		{"is_null", AssertionIsNull},
		{"is_not_null", AssertionIsNotNull},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := AssertionOperatorString(tt.input)
			if err != nil {
				t.Fatalf("expected no error for %q, got %v", tt.input, err)
			}
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestAssertionOperator_UnmarshalJSON_AcceptsAPIStrings(t *testing.T) {
	var op AssertionOperator
	if err := json.Unmarshal([]byte(`"is_not_null"`), &op); err != nil {
		t.Fatalf("expected no error unmarshaling api string, got %v", err)
	}
	if op != AssertionIsNotNull {
		t.Fatalf("expected %v, got %v", AssertionIsNotNull, op)
	}
}

func TestAssertionLogic_ToAPIString(t *testing.T) {
	tests := []struct {
		logic    AssertionLogic
		expected string
	}{
		{LogicAND, "AND"},
		{LogicOR, "OR"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.logic.ToAPIString()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestAssertionLogicFromAPIString(t *testing.T) {
	tests := []struct {
		input    string
		expected AssertionLogic
	}{
		{"AND", LogicAND},
		{"OR", LogicOR},
		{"invalid", LogicAND}, // Default case
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := AssertionLogicFromAPIString(tt.input)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
