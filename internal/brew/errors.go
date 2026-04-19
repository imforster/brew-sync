package brew

import "strings"

// BrewErrorKind classifies the type of brew install/upgrade failure.
type BrewErrorKind int

const (
	ErrUnknown         BrewErrorKind = iota
	ErrDeprecated                    // formula has been deprecated
	ErrNotFound                      // formula/cask does not exist
	ErrAlreadyInstalled              // cask app already exists on disk
)

// BrewError wraps a brew command failure with a classified kind.
type BrewError struct {
	Kind    BrewErrorKind
	Package string
	Output  string
	Wrapped error
}

func (e *BrewError) Error() string { return e.Wrapped.Error() }
func (e *BrewError) Unwrap() error { return e.Wrapped }

// ClassifyInstallError inspects brew output and returns the appropriate BrewErrorKind.
func ClassifyInstallError(output string) BrewErrorKind {
	if strings.Contains(output, "has been deprecated") {
		return ErrDeprecated
	}
	if strings.Contains(output, "No available formula with the name") ||
		strings.Contains(output, "No available cask with the name") {
		return ErrNotFound
	}
	if strings.Contains(output, "It seems there is already an App at") {
		return ErrAlreadyInstalled
	}
	return ErrUnknown
}
