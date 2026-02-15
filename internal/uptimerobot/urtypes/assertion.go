package urtypes

//go:generate go run github.com/dmarkham/enumer -type AssertionOperator -trimprefix Assertion -json -text

//+kubebuilder:validation:Type:=string
//+kubebuilder:validation:Enum:=equals;not_equals;contains;not_contains;greater_than;less_than;is_null;is_not_null

type AssertionOperator uint8

const (
	AssertionEquals AssertionOperator = iota + 1
	AssertionNotEquals
	AssertionContains
	AssertionNotContains
	AssertionGreaterThan
	AssertionLessThan
	AssertionIsNull
	AssertionIsNotNull
)

// ToAPIString returns the v3 API string representation of the assertion operator.
func (a AssertionOperator) ToAPIString() string {
	switch a {
	case AssertionEquals:
		return "equals"
	case AssertionNotEquals:
		return "not_equals"
	case AssertionContains:
		return "contains"
	case AssertionNotContains:
		return "not_contains"
	case AssertionGreaterThan:
		return "greater_than"
	case AssertionLessThan:
		return "less_than"
	case AssertionIsNull:
		return "is_null"
	case AssertionIsNotNull:
		return "is_not_null"
	default:
		return "equals"
	}
}

// AssertionOperatorFromAPIString converts a v3 API string to AssertionOperator.
func AssertionOperatorFromAPIString(s string) AssertionOperator {
	switch s {
	case "equals":
		return AssertionEquals
	case "not_equals":
		return AssertionNotEquals
	case "contains":
		return AssertionContains
	case "not_contains":
		return AssertionNotContains
	case "greater_than":
		return AssertionGreaterThan
	case "less_than":
		return AssertionLessThan
	case "is_null":
		return AssertionIsNull
	case "is_not_null":
		return AssertionIsNotNull
	default:
		return AssertionEquals
	}
}

//go:generate go run github.com/dmarkham/enumer -type AssertionLogic -trimprefix Logic -json -text

//+kubebuilder:validation:Type:=string
//+kubebuilder:validation:Enum:=AND;OR

type AssertionLogic uint8

const (
	LogicAND AssertionLogic = iota + 1
	LogicOR
)

// ToAPIString returns the v3 API string representation of the assertion logic.
func (a AssertionLogic) ToAPIString() string {
	switch a {
	case LogicAND:
		return "AND"
	case LogicOR:
		return "OR"
	default:
		return "AND"
	}
}

// AssertionLogicFromAPIString converts a v3 API string to AssertionLogic.
func AssertionLogicFromAPIString(s string) AssertionLogic {
	switch s {
	case "AND":
		return LogicAND
	case "OR":
		return LogicOR
	default:
		return LogicAND
	}
}
