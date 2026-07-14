package provider

import (
	"errors"
	"fmt"
	"regexp"
)

var autoResponderCharsetPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validateEmailAutoResponder(autoResponder EmailAutoResponderModel) error {
	if autoResponder.From.ValueString() == "" {
		return errors.New("autoresponder from name must not be empty")
	}
	if autoResponder.Subject.ValueString() == "" {
		return errors.New("autoresponder subject must not be empty")
	}
	if autoResponder.Body.ValueString() == "" {
		return errors.New("autoresponder body must not be empty")
	}
	if !autoResponderCharsetPattern.MatchString(autoResponder.Charset.ValueString()) {
		return fmt.Errorf(
			"autoresponder charset %q is invalid",
			autoResponder.Charset.ValueString(),
		)
	}

	start := autoResponder.StartUnix.ValueInt64()
	stop := autoResponder.StopUnix.ValueInt64()
	if start > 0 && stop > 0 && stop <= start {
		return errors.New(
			"autoresponder stop_unix must be greater than start_unix when both are set",
		)
	}

	return nil
}
