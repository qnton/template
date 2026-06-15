// Package validate is a tiny, dependency-free input-validation helper. Handlers
// validate user input HERE before it reaches a store or a template — never trust
// raw request data. It collects per-field messages so a form can show them all.
package validate

import (
	"net/mail"
	"strings"
	"unicode/utf8"
)

// Validator accumulates field-keyed error messages. Construct with New, run
// Check* methods, then branch on Valid.
type Validator struct {
	Errors map[string]string
}

// New returns an empty Validator.
func New() *Validator {
	return &Validator{Errors: make(map[string]string)}
}

// Valid reports whether no errors have been recorded.
func (v *Validator) Valid() bool { return len(v.Errors) == 0 }

// add records msg for field unless that field already has an error (first wins).
func (v *Validator) add(field, msg string) {
	if _, exists := v.Errors[field]; !exists {
		v.Errors[field] = msg
	}
}

// Check records msg for field when ok is false. The building block for custom rules.
func (v *Validator) Check(ok bool, field, msg string) {
	if !ok {
		v.add(field, msg)
	}
}

// Required asserts the trimmed value is non-empty.
func (v *Validator) Required(field, value string) {
	v.Check(NotBlank(value), field, "is required")
}

// MaxLen asserts the value is at most n runes long.
func (v *Validator) MaxLen(field, value string, n int) {
	v.Check(utf8.RuneCountInString(value) <= n, field, "is too long")
}

// MinLen asserts the value is at least n runes long.
func (v *Validator) MinLen(field, value string, n int) {
	v.Check(utf8.RuneCountInString(value) >= n, field, "is too short")
}

// Email asserts the value parses as an email address.
func (v *Validator) Email(field, value string) {
	_, err := mail.ParseAddress(value)
	v.Check(err == nil, field, "must be a valid email address")
}

// NotBlank reports whether s contains non-whitespace characters.
func NotBlank(s string) bool { return strings.TrimSpace(s) != "" }
