package domain

import "fmt"

type RuleError struct{ Code, Detail string }

func (e RuleError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Detail) }
func IsTerminal(s State) bool     { return s == StateArchived }

type ValidationError struct {
	Code    string
	Details any
}

func (e ValidationError) Error() string { return e.Code }
