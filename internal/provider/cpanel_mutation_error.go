package provider

import (
	"errors"
	"fmt"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
)

func sensitiveMutationError(err error, operation string) error {
	if err == nil {
		return nil
	}

	var apiError *cpanelapi.APIError
	if errors.As(err, &apiError) {
		return fmt.Errorf("cPanel rejected the %s request", operation)
	}

	var httpError *cpanelapi.HTTPError
	if errors.As(err, &httpError) {
		return fmt.Errorf(
			"cPanel returned HTTP status %d for the %s request",
			httpError.StatusCode,
			operation,
		)
	}

	return fmt.Errorf(
		"the cPanel %s request failed or returned an ambiguous response",
		operation,
	)
}

func cPanelMutationErrorIsDeterministic(err error) bool {
	var apiError *cpanelapi.APIError
	if errors.As(err, &apiError) {
		return true
	}

	var httpError *cpanelapi.HTTPError

	return errors.As(err, &httpError) &&
		httpError.StatusCode >= 400 &&
		httpError.StatusCode < 500
}

func createMutationErrorDetail(
	err error,
	operation string,
	resourceDescription string,
) string {
	redactedError := sensitiveMutationError(err, operation)
	if cPanelMutationErrorIsDeterministic(err) {
		return redactedError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform did not attempt an automatic rollback because cPanel may have applied the request before returning the error. Inspect %s and import it before retrying if it exists.",
		redactedError,
		resourceDescription,
	)
}
