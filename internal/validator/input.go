package validator

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// spaceRegexp is compiled once at package init and reused across all Sanitize calls.
var spaceRegexp = regexp.MustCompile(`\s+`)

type InputValidator struct {
	maxLength int
	minLength int
}

func NewInputValidator() *InputValidator {
	return &InputValidator{
		maxLength: 2000,
		minLength: 5,
	}
}

func (v *InputValidator) Validate(query string) error {
	if len(query) < v.minLength {
		return fmt.Errorf("query too short: minimum %d characters", v.minLength)
	}

	if len(query) > v.maxLength {
		return fmt.Errorf("query too long: maximum %d characters", v.maxLength)
	}

	if !utf8.ValidString(query) {
		return errors.New("invalid UTF-8 encoding")
	}

	if strings.TrimSpace(query) == "" {
		return errors.New("query is empty")
	}

	return nil
}

func (v *InputValidator) Sanitize(query string) string {
	query = strings.TrimSpace(query)
	query = spaceRegexp.ReplaceAllString(query, " ")
	return query
}
