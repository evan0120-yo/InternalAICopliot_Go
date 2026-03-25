package infra

import "testing"

func TestAsBusinessErrorMasksUnexpectedErrorMessage(t *testing.T) {
	err := AsBusinessError(assertionError("disk full"))
	if err.Message != "An internal error occurred." {
		t.Fatalf("expected generic internal error message, got %q", err.Message)
	}
}

type assertionError string

func (e assertionError) Error() string {
	return string(e)
}
