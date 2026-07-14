package provider

import "context"

type emailMailDomainClient interface {
	ListMailDomains(context.Context) ([]string, error)
}
