package main

// ─── Validator ────────────────────────────────────────────────────────────────

// Validator accumulates field-level validation errors.
//
// Errors is a map of field name → error message. Each field stores at most
// one message — the first failure recorded for that field wins. An empty
// Errors map means every Check passed and the input is valid.
type Validator struct {
	Errors map[string]string
}

// newValidator returns an initialised Validator ready to use.
//
// Always use newValidator() rather than &Validator{} directly.
// The zero value of a map is nil — writing to a nil map panics at runtime.
// make(map[string]string) allocates the map so it is safe to write to
// immediately.
func newValidator() *Validator {
	return &Validator{Errors: make(map[string]string)}
}

// Valid reports whether no errors have been recorded.
//
// Call this after all Check calls have run. If Valid returns false,
// at least one field failed its check and the Errors map contains
// the messages to send back to the client.
func (v *Validator) Valid() bool {
	return len(v.Errors) == 0
}

// AddError records an error message for a given field.
//
// If the field already has an error the new message is ignored — the first
// failure recorded for a field is the one that is kept. This ensures checks
// are ordered from most fundamental (required) to most specific (format), so
// the client always receives the most actionable message.
func (v *Validator) AddError(field, message string) {
	if _, exists := v.Errors[field]; !exists {
		v.Errors[field] = message
	}
}

// Check calls AddError when the ok condition is false.
//
// ok is the already-evaluated result of a boolean expression — Go evaluates
// all arguments before entering the function, so the expression is resolved
// before Check is ever called. If ok is false the check failed and the error
// is recorded against field. If ok is true nothing happens.
//
// Usage:
//
//	v.Check(input.Name != "", "name", "must be provided")
//	v.Check(len(input.Name) <= 100, "name", "must not exceed 100 characters")
//	v.Check(between(input.Year, 1, 4), "year", "must be between 1 and 4")
func (v *Validator) Check(ok bool, field, message string) {
	if !ok {
		v.AddError(field, message)
	}
}

// ─── Helper functions ─────────────────────────────────────────────────────────

// between returns true when n is within [min, max] inclusive.
//
// The type parameter [T int | float64] allows the same function to work for
// both integer and floating-point ranges without duplication.
func between[T int | float64](n, min, max T) bool {
	return n >= min && n <= max
}
