package urtypes

// API string constants for assertion operators.
const (
	APIAssertionEquals      = "equals"
	APIAssertionNotEquals   = "not_equals"
	APIAssertionContains    = "contains"
	APIAssertionNotContains = "not_contains"
	APIAssertionGreaterThan = "greater_than"
	APIAssertionLessThan    = "less_than"
	APIAssertionIsNull      = "is_null"
	APIAssertionIsNotNull   = "is_not_null"
	APIAssertionLogicAND    = "AND"
	APIAssertionLogicOR     = "OR"
)

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
		return APIAssertionEquals
	case AssertionNotEquals:
		return APIAssertionNotEquals
	case AssertionContains:
		return APIAssertionContains
	case AssertionNotContains:
		return APIAssertionNotContains
	case AssertionGreaterThan:
		return APIAssertionGreaterThan
	case AssertionLessThan:
		return APIAssertionLessThan
	case AssertionIsNull:
		return APIAssertionIsNull
	case AssertionIsNotNull:
		return APIAssertionIsNotNull
	default:
		return APIAssertionEquals
	}
}

// AssertionOperatorFromAPIString converts a v3 API string to AssertionOperator.
func AssertionOperatorFromAPIString(s string) AssertionOperator {
	switch s {
	case APIAssertionEquals:
		return AssertionEquals
	case APIAssertionNotEquals:
		return AssertionNotEquals
	case APIAssertionContains:
		return AssertionContains
	case APIAssertionNotContains:
		return AssertionNotContains
	case APIAssertionGreaterThan:
		return AssertionGreaterThan
	case APIAssertionLessThan:
		return AssertionLessThan
	case APIAssertionIsNull:
		return AssertionIsNull
	case APIAssertionIsNotNull:
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
		return APIAssertionLogicAND
	case LogicOR:
		return APIAssertionLogicOR
	default:
		return APIAssertionLogicAND
	}
}

// AssertionLogicFromAPIString converts a v3 API string to AssertionLogic.
func AssertionLogicFromAPIString(s string) AssertionLogic {
	switch s {
	case APIAssertionLogicAND:
		return LogicAND
	case APIAssertionLogicOR:
		return LogicOR
	default:
		return LogicAND
	}
}
