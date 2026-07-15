package sslcertificate

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"terraform-provider-cpanel/internal/cpanel"
)

const (
	moduleSSL                   = "SSL"
	operationListCertificates   = "list_certs"
	operationShowCertificate    = "show_cert"
	operationUploadCertificate  = "upload_cert"
	operationRenameCertificate  = "set_cert_friendly_name"
	operationDeleteCertificate  = "delete_cert"
	operationListInstalledHosts = "installed_hosts"
)

// Client manages public SSL certificates stored by cPanel.
type Client struct {
	*cpanel.Client
}

// NewClient creates an SSL certificate client from the shared cPanel client.
func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

// List returns the complete cPanel certificate inventory sorted by ID.
func (c *Client) List(ctx context.Context) ([]Certificate, error) {
	response := listResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		moduleSSL,
		operationListCertificates,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	apiCertificates, err := parseRequiredArray[apiCertificate](
		response.Data,
		"SSL certificate inventory data",
	)
	if err != nil {
		return nil, err
	}

	certificates := make([]Certificate, 0, len(apiCertificates))
	seen := make(map[string]struct{}, len(apiCertificates))
	for _, apiCertificate := range apiCertificates {
		certificate, err := certificateFromAPI(apiCertificate, true)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[certificate.ID]; exists {
			return nil, fmt.Errorf(
				"cPanel returned duplicate SSL certificate id %q",
				certificate.ID,
			)
		}
		seen[certificate.ID] = struct{}{}
		certificates = append(certificates, certificate)
	}
	sort.Slice(certificates, func(left, right int) bool {
		return certificates[left].ID < certificates[right].ID
	})

	return certificates, nil
}

// Get returns a certificate by its canonical cPanel ID.
func (c *Client) Get(
	ctx context.Context,
	id string,
) (*Certificate, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}

	certificates, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	index := sort.Search(len(certificates), func(index int) bool {
		return certificates[index].ID >= id
	})
	if index == len(certificates) || certificates[index].ID != id {
		return nil, nil
	}

	response := showResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		moduleSSL,
		operationShowCertificate,
		map[string]string{"id": id},
		&response,
	); err != nil {
		return nil, err
	}

	details, err := certificateFromAPI(response.Data.Details, false)
	if err != nil {
		return nil, fmt.Errorf(
			"decode SSL certificate %q details: %w",
			id,
			err,
		)
	}
	if details.ID != id {
		return nil, fmt.Errorf(
			"cPanel show_cert returned certificate id %q for requested id %q",
			details.ID,
			id,
		)
	}

	certificatePEM, err := parseScalarString(
		response.Data.CertificatePEM,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"decode SSL certificate %q PEM: %w",
			id,
			err,
		)
	}
	parsed, err := ParsePEM(certificatePEM)
	if err != nil {
		return nil, fmt.Errorf(
			"validate SSL certificate %q PEM: %w",
			id,
			err,
		)
	}

	certificate := certificates[index]
	certificate.FriendlyName = details.FriendlyName
	certificate.Domains = details.Domains
	certificate.NotBefore = details.NotBefore
	certificate.NotAfter = details.NotAfter
	certificate.KeyAlgorithm = details.KeyAlgorithm
	certificate.ModulusLength = details.ModulusLength
	certificate.IsSelfSigned = details.IsSelfSigned
	certificate.IssuerCommonName = details.IssuerCommonName
	certificate.SubjectCommonName = details.SubjectCommonName
	certificate.CertificatePEM = parsed.NormalizedPEM
	certificate.FingerprintSHA256 = parsed.SHA256Fingerprint

	return &certificate, nil
}

// Upload stores one validated public certificate in cPanel.
func (c *Client) Upload(
	ctx context.Context,
	certificatePEM string,
	friendlyName string,
) (*Certificate, error) {
	parsed, err := ParsePEM(certificatePEM)
	if err != nil {
		return nil, err
	}
	if err := ValidateFriendlyName(friendlyName); err != nil {
		return nil, err
	}

	response := uploadResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		moduleSSL,
		operationUploadCertificate,
		map[string]string{
			"crt":           parsed.NormalizedPEM,
			"friendly_name": friendlyName,
		},
		&response,
	); err != nil {
		return nil, err
	}
	apiCertificates, err := parseRequiredArray[apiCertificate](
		response.Data,
		"SSL certificate upload data",
	)
	if err != nil {
		return nil, err
	}
	if len(apiCertificates) != 1 {
		return nil, fmt.Errorf(
			"cPanel SSL certificate upload returned %d certificates; expected exactly one",
			len(apiCertificates),
		)
	}

	certificate, err := certificateFromAPI(apiCertificates[0], false)
	if err != nil {
		return nil, err
	}
	certificate.CertificatePEM = parsed.NormalizedPEM
	certificate.FingerprintSHA256 = parsed.SHA256Fingerprint

	return &certificate, nil
}

// Rename updates a stored certificate's cPanel friendly name.
func (c *Client) Rename(
	ctx context.Context,
	id string,
	newFriendlyName string,
) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	if err := ValidateFriendlyName(newFriendlyName); err != nil {
		return err
	}

	response := mutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		moduleSSL,
		operationRenameCertificate,
		map[string]string{
			"id":                id,
			"new_friendly_name": newFriendlyName,
		},
		&response,
	)
}

// Delete removes a stored certificate by its cPanel ID.
func (c *Client) Delete(ctx context.Context, id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}

	response := mutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		moduleSSL,
		operationDeleteCertificate,
		map[string]string{"id": id},
		&response,
	)
}

// IsInstalled reports whether an installed SSL host references the certificate.
func (c *Client) IsInstalled(
	ctx context.Context,
	id string,
) (bool, error) {
	if err := ValidateID(id); err != nil {
		return false, err
	}

	response := installedHostsResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		moduleSSL,
		operationListInstalledHosts,
		map[string]string{},
		&response,
	); err != nil {
		return false, err
	}

	hosts, err := parseRequiredArray[apiInstalledHost](
		response.Data,
		"installed SSL host inventory data",
	)
	if err != nil {
		return false, err
	}

	for index, host := range hosts {
		if host.Certificate == nil {
			return false, fmt.Errorf(
				"installed SSL host %d has no certificate object",
				index,
			)
		}
		installedID, err := parseScalarString(host.Certificate.ID)
		if err != nil {
			return false, fmt.Errorf(
				"decode installed SSL certificate id: %w",
				err,
			)
		}
		if installedID == "" {
			return false, fmt.Errorf(
				"installed SSL host %d has no certificate id",
				index,
			)
		}
		if err := ValidateID(installedID); err != nil {
			return false, fmt.Errorf(
				"invalid installed SSL certificate id: %w",
				err,
			)
		}
		if installedID == id {
			return true, nil
		}
	}

	return false, nil
}
