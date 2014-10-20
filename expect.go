package expect

import (
	"bytes"
	"reflect"
	"strings"
)

type Expectation struct {
	actual         interface{}
	others         []interface{}
	Greater        *ThanAssertion
	GreaterOrEqual *ToAssertion
	Less           *ThanAssertion
	LessOrEqual    *ToAssertion
	To             *ToExpectation
	Not            *InvertedExpectation
}

func (e *Expectation) ToEqual(expected interface{}, others ...interface{}) PostHandler {
	return e.To.Equal(expected, others...)
}

func (e *Expectation) GreaterThan(expected interface{}) PostHandler {
	return e.Greater.Than(expected)
}

func (e *Expectation) GreaterOrEqualTo(expected interface{}) PostHandler {
	return e.GreaterOrEqual.To(expected)
}

func (e *Expectation) LessThan(expected interface{}) PostHandler {
	return e.Less.Than(expected)
}

func (e *Expectation) LessOrEqualTo(expected interface{}) PostHandler {
	return e.LessOrEqual.To(expected)
}

type InvertedExpectation struct {
	*Expectation
}

var Errorf = func(format string, args ...interface{}) {
	runner.Errorf(format, args...)
}

func Expect(actual interface{}, others ...interface{}) *Expectation {
	return expect(actual, others, true)
}

func Fail(format string, args ...interface{}) {
	Errorf(format, args...)
}

func Skip(format string, args ...interface{}) {
	runner.Skip(format, args...)
}

func expect(actual interface{}, others []interface{}, includeNot bool) *Expectation {
	e := &Expectation{actual: actual, others: others}
	e.Greater = newThanAssertion(actual, GreaterThanComparitor, "to be greater than", "greater than")
	e.GreaterOrEqual = newToAssertion(actual, GreaterOrEqualToComparitor, "to be greater or equal to")
	e.Less = newThanAssertion(actual, LessThanComparitor, "to be less than", "less than")
	e.LessOrEqual = newToAssertion(actual, LessThanOrEqualToComparitor, "to be less or equal to")
	e.To = &ToExpectation{
		actual: actual,
		others: others,
	}
	if includeNot {
		e.Not = NotExpect(actual, others...)
	}
	return e
}

func NotExpect(actual interface{}, others ...interface{}) *InvertedExpectation {
	e := &InvertedExpectation{expect(actual, others, false)}
	e.Greater.invert = true
	e.GreaterOrEqual.invert = true
	e.Less.invert = true
	e.LessOrEqual.invert = true
	e.To.invert = true
	return e
}

type ToExpectation struct {
	invert bool
	actual interface{}
	others []interface{}
}

func (e *ToExpectation) Equal(expected interface{}, others ...interface{}) PostHandler {
	display := "to be equal to"
	if e.invert {
		display = "to equal"
	}
	assertion := newToAssertion(e.actual, EqualsComparitor, display)
	assertion.invert = e.invert
	failed := !equal(assertion, e.actual, expected)

	if len(others) != len(e.others) {
		Errorf("mismatch number of values and expectations %d != %d", len(e.others)+1, len(others)+1)
		failed = true
	} else {
		for i := 0; i < len(others); i++ {
			if equal(assertion, e.others[i], others[i]) == false {
				failed = true
			}
		}
	}

	if failed {
		return FailureHandler
	}
	return SuccessHandler
}

func equal(assertion *ToAssertion, a, b interface{}) bool {
	aIsNil := IsNil(a)
	bIsNil := IsNil(b)
	if aIsNil || bIsNil {
		if (aIsNil == bIsNil) == assertion.invert {
			showError(a, b, assertion.invert, assertion.display)
			return false
		}
		return true
	}
	assertion.actual = a
	return assertion.To(b) == SuccessHandler
}

func (e *ToExpectation) Contain(expected interface{}) PostHandler {
	c := contains(e.actual, expected)
	if e.invert == false && c == false {
		Errorf("%v does not contain %v", e.actual, expected)
		return FailureHandler
	}
	if e.invert == true && c == true {
		Errorf("%v contains %v", e.actual, expected)
		return FailureHandler
	}
	return SuccessHandler
}

type ToAssertion struct {
	actual     interface{}
	comparitor Comparitor
	display    string
	invert     bool
}

func newToAssertion(a interface{}, c Comparitor, display string) *ToAssertion {
	return &ToAssertion{
		actual:     a,
		comparitor: c,
		display:    display,
	}
}

func (a *ToAssertion) To(expected interface{}) PostHandler {
	actual := a.actual
	kind, ok := SameKind(actual, expected)
	if ok == false {
		Errorf("expected %v %s %v - type mismatch %s != %s", actual, a.display, expected, reflect.ValueOf(actual).Kind(), reflect.ValueOf(expected).Kind())
		return FailureHandler
	}
	if IsInt(actual) {
		actual, expected = ToInt64(actual, expected)
		kind = reflect.Int64
	} else if IsUint(actual) {
		actual, expected = ToUint64(actual, expected)
		kind = reflect.Uint64
	}
	if a.comparitor(kind, actual, expected) == a.invert {
		showError(actual, expected, a.invert, a.display)
		return FailureHandler
	}
	return SuccessHandler
}

func showError(actual, expected interface{}, invert bool, display string) {
	var inversion string
	if invert {
		inversion = "not "
	}
	Errorf("expected %v %s%s %v", actual, inversion, display, expected)
}

type ThanAssertion struct {
	to      *ToAssertion
	display string
	invert  bool
}

func newThanAssertion(actual interface{}, c Comparitor, toDisplay, thanDisplay string) *ThanAssertion {
	return &ThanAssertion{
		to:      newToAssertion(actual, c, toDisplay),
		display: thanDisplay,
	}
}

func (a *ThanAssertion) Than(expected interface{}) PostHandler {
	actual := a.to.actual
	a.to.invert = a.invert
	if IsNumeric(actual) == false {
		Errorf("cannot use %s for type %s", a.display, reflect.ValueOf(actual).Kind())
		return FailureHandler
	}
	if IsNumeric(expected) == false {
		Errorf("cannot use %s for type %s", a.display, reflect.ValueOf(expected).Kind())
		return FailureHandler
	}
	return a.to.To(expected)
}

func contains(actual, expected interface{}) bool {
	actualValue, expectedValue := reflect.ValueOf(actual), reflect.ValueOf(expected)
	actualKind, expectedKind := actualValue.Kind(), expectedValue.Kind()
	if actualKind == reflect.String {
		if expectedKind == reflect.String && strings.Contains(actual.(string), expected.(string)) {
			return true
		}
		return false
	}
	if actualKind == reflect.Slice || actualKind == reflect.Array {
		for i, l := 0, actualValue.Len(); i < l; i++ {
			if reflect.DeepEqual(actualValue.Index(i).Interface(), expected) {
				return true
			}
		}
	}
	if actualKind == reflect.Map {
		return actualValue.MapIndex(expectedValue).Kind() != reflect.Invalid
	}
	if actualBytes, ok := actual.([]byte); ok {
		if expectedBytes, ok := expected.([]byte); ok {
			return bytes.Contains(actualBytes, expectedBytes)
		}
	}
	return false
}
