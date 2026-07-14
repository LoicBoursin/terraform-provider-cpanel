package provider

import "errors"

func reconcileRenamePresence(
	oldExists bool,
	newExists bool,
) (bool, error) {
	switch {
	case !oldExists && newExists:
		return false, errors.New(
			"the old name is absent and the new name is present, but cPanel does not expose enough information to prove that the new object is the renamed Terraform object",
		)
	case oldExists && !newExists:
		return false, nil
	case oldExists && newExists:
		return false, errors.New(
			"both the old and new names exist after the rename request",
		)
	default:
		return false, errors.New(
			"neither the old nor new name exists after the rename request",
		)
	}
}
