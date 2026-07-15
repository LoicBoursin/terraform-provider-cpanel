package boxtrapper

import "fmt"

// MutationVerificationError means cPanel accepted a mutation request, but the
// provider could not prove the resulting remote state.
type MutationVerificationError struct {
	Operation string
	Err       error
}

func (e *MutationVerificationError) Error() string {
	return fmt.Sprintf(
		"verify BoxTrapper %s mutation: %v",
		e.Operation,
		e.Err,
	)
}

func (e *MutationVerificationError) Unwrap() error {
	return e.Err
}
