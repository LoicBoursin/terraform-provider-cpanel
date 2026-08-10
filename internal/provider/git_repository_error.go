package provider

func gitRepositoryReadError(err error) error {
	return sensitiveMutationError(err, "Git repository inventory read")
}
