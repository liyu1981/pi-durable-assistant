// Package sqlutil formats the few SQL fragments that cannot be bound as
// parameters (numeric schema versions and identifiers). Values must always be
// passed through database/sql placeholders, never through Format.
package sqlutil

import (
	"fmt"
	"regexp"
)

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Ident is a validated SQL identifier such as a table or column name.
type Ident string

// Int is a validated integer literal for clauses that do not accept a bind
// parameter, such as PRAGMA user_version.
type Int int64

// Format builds a SQL fragment from a template. Only Ident and Int values are
// accepted; a plain string is rejected so user input cannot be interpolated.
func Format(format string, args ...any) (string, error) {
	for i, arg := range args {
		switch value := arg.(type) {
		case Ident:
			if !identifier.MatchString(string(value)) {
				return "", fmt.Errorf("sql argument %d is not a safe identifier", i)
			}
		case Int:
		default:
			return "", fmt.Errorf("sql argument %d has unsupported type %T", i, arg)
		}
	}
	return fmt.Sprintf(format, args...), nil
}
