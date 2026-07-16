package sslcsr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"terraform-provider-cpanel/internal/cpanel"
)

const (
	defaultGenerationRecoveryTimeout      = 30 * time.Second
	defaultGenerationRecoveryPollInterval = 500 * time.Millisecond
)

// Client manages certificate signing requests stored by cPanel.
type Client struct {
	*cpanel.Client

	mutationMu              sync.Mutex
	generationRecoveryDelay time.Duration
	generationRecoveryLimit time.Duration
}

// NewClient creates an SSL CSR client from the shared cPanel client.
func NewClient(client *cpanel.Client) *Client {
	return &Client{
		Client:                  client,
		generationRecoveryDelay: defaultGenerationRecoveryPollInterval,
		generationRecoveryLimit: defaultGenerationRecoveryTimeout,
	}
}

// List returns the complete CSR inventory with canonical PEM fingerprints.
func (c *Client) List(ctx context.Context) ([]CSR, error) {
	inventory, err := c.listMetadata(ctx)
	if err != nil {
		return nil, err
	}

	csrs := make([]CSR, 0, len(inventory))
	for _, metadata := range inventory {
		csr, err := c.show(ctx, metadata)
		if err != nil {
			return nil, err
		}
		csrs = append(csrs, *csr)
	}

	return csrs, nil
}

// ListMetadata returns safe CSR metadata without fetching CSR PEM.
func (c *Client) ListMetadata(
	ctx context.Context,
) ([]CSRMetadata, error) {
	apiCSRs, err := c.listAPIInventory(ctx)
	if err != nil {
		return nil, err
	}
	inventory := make([]CSRMetadata, 0, len(apiCSRs))
	seen := make(map[string]struct{}, len(apiCSRs))
	for _, apiCSR := range apiCSRs {
		csr, err := csrMetadataFromAPI(apiCSR)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[csr.ID]; exists {
			return nil, fmt.Errorf(
				"cPanel returned duplicate SSL CSR id %q",
				csr.ID,
			)
		}
		seen[csr.ID] = struct{}{}
		inventory = append(inventory, csr)
	}
	sort.Slice(inventory, func(left, right int) bool {
		return inventory[left].ID < inventory[right].ID
	})

	return inventory, nil
}

// ListKeys returns safe public metadata for stored SSL keys.
func (c *Client) ListKeys(ctx context.Context) ([]KeyMetadata, error) {
	apiKeys, err := c.listAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]KeyMetadata, 0, len(apiKeys))
	seen := make(map[string]struct{}, len(apiKeys))
	for _, apiKey := range apiKeys {
		key, err := keyMetadataFromAPI(apiKey)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[key.ID]; exists {
			return nil, fmt.Errorf(
				"cPanel returned duplicate SSL key id %q",
				key.ID,
			)
		}
		seen[key.ID] = struct{}{}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		return keys[left].ID < keys[right].ID
	})

	return keys, nil
}

// Get returns one CSR by its canonical cPanel ID.
func (c *Client) Get(ctx context.Context, id string) (*CSR, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}

	inventory, err := c.listMetadata(ctx)
	if err != nil {
		return nil, err
	}
	index := sort.Search(len(inventory), func(index int) bool {
		return inventory[index].ID >= id
	})
	if index == len(inventory) || inventory[index].ID != id {
		return nil, nil
	}

	return c.show(ctx, inventory[index])
}

// Generate creates and verifies a CSR from an existing cPanel key.
func (c *Client) Generate(
	ctx context.Context,
	definition Definition,
) (*CSR, error) {
	if err := ValidateDefinition(definition); err != nil {
		return nil, err
	}

	c.mutationMu.Lock()
	defer c.mutationMu.Unlock()

	selectedKey, err := c.getPublicKey(ctx, definition.KeyID)
	if err != nil {
		return nil, fmt.Errorf(
			"read selected SSL key %q before CSR generation: %w",
			definition.KeyID,
			err,
		)
	}
	if selectedKey == nil {
		return nil, fmt.Errorf(
			"selected SSL key %q was not found before CSR generation",
			definition.KeyID,
		)
	}

	baseline, err := c.listMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"inventory SSL CSRs before generation: %w",
			err,
		)
	}
	baselineIDs := make(map[string]struct{}, len(baseline))
	for _, csr := range baseline {
		baselineIDs[csr.ID] = struct{}{}
	}

	response := mutationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		moduleSSL,
		operationGenerateCSR,
		map[string]string{
			"countryName":            definition.CountryName,
			"domains":                strings.Join(definition.Domains, ","),
			"emailAddress":           definition.EmailAddress,
			"friendly_name":          definition.FriendlyName,
			"key_id":                 definition.KeyID,
			"localityName":           definition.LocalityName,
			"organizationName":       definition.OrganizationName,
			"organizationalUnitName": definition.OrganizationalUnitName,
			"stateOrProvinceName":    definition.StateOrProvinceName,
		},
		&response,
	); err != nil {
		if isDeterministicMutationError(err) {
			return nil, err
		}

		return c.recoverGeneratedCSR(
			ctx,
			baselineIDs,
			definition,
			*selectedKey,
			fmt.Errorf("generate SSL CSR: %w", err),
		)
	}

	apiGenerated, err := parseRequiredObject[apiCSR](
		response.Data,
		"SSL CSR generation data",
	)
	if err != nil {
		return c.recoverGeneratedCSR(
			ctx,
			baselineIDs,
			definition,
			*selectedKey,
			err,
		)
	}
	generated, err := csrFromAPI(apiGenerated)
	if err != nil {
		return c.recoverGeneratedCSR(
			ctx,
			baselineIDs,
			definition,
			*selectedKey,
			fmt.Errorf("decode generated SSL CSR: %w", err),
		)
	}
	generatedPEM, err := parseRequiredScalarString(apiGenerated.Text)
	if err != nil {
		return c.recoverGeneratedCSR(
			ctx,
			baselineIDs,
			definition,
			*selectedKey,
			fmt.Errorf(
				"decode generated SSL CSR %q PEM: %w",
				generated.ID,
				err,
			),
		)
	}
	generated.CountryName = definition.CountryName
	generated.StateOrProvinceName = definition.StateOrProvinceName
	generated.LocalityName = definition.LocalityName
	generated.OrganizationName = definition.OrganizationName
	generated.OrganizationalUnitName = definition.OrganizationalUnitName
	generated.EmailAddress = definition.EmailAddress
	generated, err = attachPEM(generated, generatedPEM)
	if err != nil {
		return c.recoverGeneratedCSR(
			ctx,
			baselineIDs,
			definition,
			*selectedKey,
			fmt.Errorf(
				"validate generated SSL CSR %q PEM: %w",
				generated.ID,
				err,
			),
		)
	}
	if _, existed := baselineIDs[generated.ID]; existed {
		return c.recoverGeneratedCSR(
			ctx,
			baselineIDs,
			definition,
			*selectedKey,
			fmt.Errorf(
				"cPanel returned existing SSL CSR id %q after generation",
				generated.ID,
			),
		)
	}

	actual, err := c.Get(ctx, generated.ID)
	if err != nil {
		return c.recoverGeneratedCSR(
			ctx,
			baselineIDs,
			definition,
			*selectedKey,
			fmt.Errorf(
				"read SSL CSR %q after generation: %w",
				generated.ID,
				err,
			),
		)
	}
	if actual == nil {
		return c.recoverGeneratedCSR(
			ctx,
			baselineIDs,
			definition,
			*selectedKey,
			fmt.Errorf(
				"SSL CSR %q was not found after generation",
				generated.ID,
			),
		)
	}
	if err := verifyInventoryMetadata(generated, *actual); err != nil {
		return c.recoverGeneratedCSR(
			ctx,
			baselineIDs,
			definition,
			*selectedKey,
			fmt.Errorf(
				"verify SSL CSR %q generation response: %w",
				generated.ID,
				err,
			),
		)
	}
	if generated.FingerprintSHA256 != actual.FingerprintSHA256 {
		return c.recoverGeneratedCSR(
			ctx,
			baselineIDs,
			definition,
			*selectedKey,
			fmt.Errorf(
				"SSL CSR %q fingerprint is %q after generation; expected %q",
				generated.ID,
				actual.FingerprintSHA256,
				generated.FingerprintSHA256,
			),
		)
	}
	if err := verifyDefinition(*actual, definition); err != nil {
		return c.recoverGeneratedCSR(
			ctx,
			baselineIDs,
			definition,
			*selectedKey,
			fmt.Errorf(
				"verify SSL CSR %q after generation: %w",
				generated.ID,
				err,
			),
		)
	}
	if err := verifyCSRUsesPublicKey(*actual, *selectedKey); err != nil {
		return c.recoverGeneratedCSR(
			ctx,
			baselineIDs,
			definition,
			*selectedKey,
			fmt.Errorf(
				"verify SSL CSR %q selected key: %w",
				generated.ID,
				err,
			),
		)
	}

	return actual, nil
}

// Rename changes and verifies a CSR friendly name without changing identity.
func (c *Client) Rename(
	ctx context.Context,
	identity Identity,
	expectedFriendlyName string,
	newFriendlyName string,
) (*CSR, error) {
	if err := ValidateIdentity(identity); err != nil {
		return nil, err
	}
	if expectedFriendlyName == "" {
		return nil, fmt.Errorf(
			"expected SSL CSR friendly name must not be empty",
		)
	}
	if err := ValidateFriendlyName(expectedFriendlyName); err != nil {
		return nil, err
	}
	if newFriendlyName == "" {
		return nil, fmt.Errorf("SSL CSR friendly name must not be empty")
	}
	if err := ValidateFriendlyName(newFriendlyName); err != nil {
		return nil, err
	}

	c.mutationMu.Lock()
	defer c.mutationMu.Unlock()

	current, err := c.Get(ctx, identity.ID)
	if err != nil {
		return nil, fmt.Errorf(
			"read SSL CSR %q before rename: %w",
			identity.ID,
			err,
		)
	}
	if current == nil {
		return nil, fmt.Errorf(
			"SSL CSR %q was not found before rename",
			identity.ID,
		)
	}
	if err := verifyIdentity(*current, identity); err != nil {
		return nil, err
	}
	if current.FriendlyName != expectedFriendlyName {
		return nil, fmt.Errorf(
			"SSL CSR %q friendly name is %q; expected %q before rename",
			identity.ID,
			current.FriendlyName,
			expectedFriendlyName,
		)
	}
	if current.FriendlyName == newFriendlyName {
		return current, nil
	}

	response := mutationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		moduleSSL,
		operationRenameCSR,
		map[string]string{
			"friendly_name":     current.FriendlyName,
			"id":                identity.ID,
			"new_friendly_name": newFriendlyName,
		},
		&response,
	); err != nil {
		return nil, err
	}

	updated, err := c.Get(ctx, identity.ID)
	if err != nil {
		return nil, fmt.Errorf(
			"read SSL CSR %q after rename: %w",
			identity.ID,
			err,
		)
	}
	if updated == nil {
		return nil, fmt.Errorf(
			"SSL CSR %q was not found after rename",
			identity.ID,
		)
	}
	if err := verifyIdentity(*updated, identity); err != nil {
		return nil, err
	}
	if updated.FriendlyName != newFriendlyName {
		return nil, fmt.Errorf(
			"SSL CSR %q friendly name is %q after rename; expected %q",
			identity.ID,
			updated.FriendlyName,
			newFriendlyName,
		)
	}
	if err := verifyCSRUnchanged(*current, *updated); err != nil {
		return nil, fmt.Errorf(
			"verify SSL CSR %q after rename: %w",
			identity.ID,
			err,
		)
	}

	return updated, nil
}

// Delete removes only the exact CSR identified by its ID and PEM fingerprint.
func (c *Client) Delete(
	ctx context.Context,
	identity Identity,
) error {
	if err := ValidateIdentity(identity); err != nil {
		return err
	}

	c.mutationMu.Lock()
	defer c.mutationMu.Unlock()

	current, err := c.Get(ctx, identity.ID)
	if err != nil {
		return fmt.Errorf(
			"read SSL CSR %q before deletion: %w",
			identity.ID,
			err,
		)
	}
	if current == nil {
		return nil
	}
	if err := verifyIdentity(*current, identity); err != nil {
		return err
	}

	response := mutationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		moduleSSL,
		operationDeleteCSR,
		map[string]string{
			"friendly_name": current.FriendlyName,
			"id":            identity.ID,
		},
		&response,
	); err != nil {
		return err
	}

	remaining, err := c.Get(ctx, identity.ID)
	if err != nil {
		return fmt.Errorf(
			"read SSL CSR %q after deletion: %w",
			identity.ID,
			err,
		)
	}
	if remaining != nil {
		return fmt.Errorf(
			"SSL CSR %q still exists after deletion",
			identity.ID,
		)
	}

	return nil
}

func (c *Client) listPublicKeys(
	ctx context.Context,
) ([]publicKeyMetadata, error) {
	apiKeys, err := c.listAPIKeys(ctx)
	if err != nil {
		return nil, err
	}

	keys := make([]publicKeyMetadata, 0, len(apiKeys))
	seen := make(map[string]struct{}, len(apiKeys))
	for _, apiKey := range apiKeys {
		key, err := publicKeyFromAPI(apiKey)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[key.ID]; exists {
			return nil, fmt.Errorf(
				"cPanel returned duplicate SSL public key id %q",
				key.ID,
			)
		}
		seen[key.ID] = struct{}{}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		return keys[left].ID < keys[right].ID
	})

	return keys, nil
}

func (c *Client) getPublicKey(
	ctx context.Context,
	id string,
) (*publicKeyMetadata, error) {
	keys, err := c.listPublicKeys(ctx)
	if err != nil {
		return nil, err
	}
	index := sort.Search(len(keys), func(index int) bool {
		return keys[index].ID >= id
	})
	if index == len(keys) || keys[index].ID != id {
		return nil, nil
	}

	return &keys[index], nil
}

func (c *Client) recoverGeneratedCSR(
	ctx context.Context,
	baselineIDs map[string]struct{},
	definition Definition,
	selectedKey publicKeyMetadata,
	cause error,
) (*CSR, error) {
	recoveryLimit := c.generationRecoveryLimit
	if recoveryLimit <= 0 {
		recoveryLimit = defaultGenerationRecoveryTimeout
	}
	recoveryDelay := c.generationRecoveryDelay
	if recoveryDelay <= 0 {
		recoveryDelay = defaultGenerationRecoveryPollInterval
	}
	recoveryContext, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		recoveryLimit,
	)
	defer cancel()

	var lastObservation string
	for {
		recovered, observation, err := c.observeGeneratedCSR(
			recoveryContext,
			baselineIDs,
			definition,
			selectedKey,
		)
		if err != nil {
			return nil, fmt.Errorf("%v; %w", cause, err)
		}
		if recovered != nil {
			return recovered, nil
		}
		lastObservation = observation

		select {
		case <-recoveryContext.Done():
			return nil, fmt.Errorf(
				"%v; unable to verify a newly created SSL CSR within %s: %s",
				cause,
				recoveryLimit,
				lastObservation,
			)
		case <-time.After(recoveryDelay):
		}
	}
}

func (c *Client) observeGeneratedCSR(
	ctx context.Context,
	baselineIDs map[string]struct{},
	definition Definition,
	selectedKey publicKeyMetadata,
) (*CSR, string, error) {
	inventory, err := c.listMetadata(ctx)
	if err != nil {
		return nil, fmt.Sprintf(
			"latest SSL CSR inventory read failed: %v",
			err,
		), nil
	}

	matches := make([]CSR, 0, 1)
	plausibleCount := 0
	unresolvedCount := 0
	verificationFailure := ""
	for _, metadata := range inventory {
		if _, existed := baselineIDs[metadata.ID]; existed {
			continue
		}
		if !csrMetadataMatchesDefinition(metadata, definition) {
			continue
		}
		plausibleCount++

		candidate, err := c.show(ctx, metadata)
		if err != nil {
			unresolvedCount++
			verificationFailure = fmt.Sprintf(
				"newly observed SSL CSR %q could not be read: %v",
				metadata.ID,
				err,
			)
			continue
		}
		if err := verifyDefinition(*candidate, definition); err != nil {
			verificationFailure = fmt.Sprintf(
				"newly observed SSL CSR %q did not match the requested definition: %v",
				metadata.ID,
				err,
			)
			continue
		}
		if err := verifyCSRUsesPublicKey(
			*candidate,
			selectedKey,
		); err != nil {
			verificationFailure = fmt.Sprintf(
				"newly observed SSL CSR %q did not match selected key %q: %v",
				metadata.ID,
				selectedKey.ID,
				err,
			)
			continue
		}
		matches = append(matches, *candidate)
	}

	switch len(matches) {
	case 1:
		if unresolvedCount != 0 {
			return nil, fmt.Sprintf(
				"one newly observed SSL CSR matched exactly, but %d other plausible CSR(s) could not yet be read",
				unresolvedCount,
			), nil
		}

		return &matches[0], "", nil
	case 0:
		if plausibleCount == 0 {
			return nil,
				"no new SSL CSR matching the requested visible metadata has appeared",
				nil
		}
		if verificationFailure != "" {
			return nil, verificationFailure, nil
		}

		return nil, fmt.Sprintf(
			"%d new SSL CSR(s) matched the visible request metadata but none could be verified against selected key %q",
			plausibleCount,
			selectedKey.ID,
		), nil
	default:
		return nil, "", fmt.Errorf(
			"refuse to adopt any CSR because %d newly created CSRs exactly match the request and selected key %q",
			len(matches),
			selectedKey.ID,
		)
	}
}

func csrMetadataMatchesDefinition(
	metadata CSR,
	definition Definition,
) bool {
	if metadata.FriendlyName != definition.FriendlyName ||
		metadata.CommonName != definition.Domains[0] {
		return false
	}
	expectedDomains := append([]string(nil), definition.Domains...)
	sort.Strings(expectedDomains)

	return equalStrings(metadata.Domains, expectedDomains)
}

func isDeterministicMutationError(err error) bool {
	var apiError *cpanel.APIError

	return errors.As(err, &apiError)
}

func (c *Client) listMetadata(ctx context.Context) ([]CSR, error) {
	apiCSRs, err := c.listAPIInventory(ctx)
	if err != nil {
		return nil, err
	}

	inventory := make([]CSR, 0, len(apiCSRs))
	seen := make(map[string]struct{}, len(apiCSRs))
	for _, apiCSR := range apiCSRs {
		csr, err := csrFromAPI(apiCSR)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[csr.ID]; exists {
			return nil, fmt.Errorf(
				"cPanel returned duplicate SSL CSR id %q",
				csr.ID,
			)
		}
		seen[csr.ID] = struct{}{}
		inventory = append(inventory, csr)
	}
	sort.Slice(inventory, func(left, right int) bool {
		return inventory[left].ID < inventory[right].ID
	})

	return inventory, nil
}

func (c *Client) listAPIInventory(
	ctx context.Context,
) ([]apiCSR, error) {
	response := listResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		moduleSSL,
		operationListCSRs,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	apiCSRs, err := parseRequiredArray[apiCSR](
		response.Data,
		"SSL CSR inventory data",
	)
	if err != nil {
		return nil, err
	}

	return apiCSRs, nil
}

func (c *Client) listAPIKeys(
	ctx context.Context,
) ([]apiPublicKey, error) {
	response := keyListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		moduleSSL,
		operationListKeys,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return parseRequiredArray[apiPublicKey](
		response.Data,
		"SSL public key inventory data",
	)
}

func (c *Client) show(
	ctx context.Context,
	inventory CSR,
) (*CSR, error) {
	response := showResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		moduleSSL,
		operationShowCSR,
		map[string]string{"id": inventory.ID},
		&response,
	); err != nil {
		return nil, err
	}

	data, err := parseRequiredObject[apiShowCSR](
		response.Data,
		fmt.Sprintf("SSL CSR %q show data", inventory.ID),
	)
	if err != nil {
		return nil, err
	}
	apiDetails, err := parseRequiredObject[apiCSR](
		data.Details,
		fmt.Sprintf("SSL CSR %q details", inventory.ID),
	)
	if err != nil {
		return nil, err
	}
	details, err := csrFromAPI(apiDetails)
	if err != nil {
		return nil, fmt.Errorf(
			"decode SSL CSR %q details: %w",
			inventory.ID,
			err,
		)
	}
	if err := verifyShowMetadata(inventory, details); err != nil {
		return nil, fmt.Errorf(
			"verify SSL CSR %q list and show metadata: %w",
			inventory.ID,
			err,
		)
	}

	csrPEM, err := parseRequiredScalarString(data.CSR)
	if err != nil {
		return nil, fmt.Errorf(
			"decode SSL CSR %q PEM: %w",
			inventory.ID,
			err,
		)
	}

	merged := inventory
	merged.CountryName = details.CountryName
	merged.StateOrProvinceName = details.StateOrProvinceName
	merged.LocalityName = details.LocalityName
	merged.OrganizationName = details.OrganizationName
	merged.OrganizationalUnitName = details.OrganizationalUnitName
	merged.EmailAddress = details.EmailAddress
	merged, err = attachPEM(merged, csrPEM)
	if err != nil {
		return nil, fmt.Errorf(
			"validate SSL CSR %q PEM: %w",
			inventory.ID,
			err,
		)
	}

	return &merged, nil
}

func verifyShowMetadata(expected, actual CSR) error {
	checks := []struct {
		field    string
		expected string
		actual   string
	}{
		{"id", expected.ID, actual.ID},
		{"friendly_name", expected.FriendlyName, actual.FriendlyName},
		{"commonName", expected.CommonName, actual.CommonName},
		{"key_algorithm", expected.KeyAlgorithm, actual.KeyAlgorithm},
	}
	for _, check := range checks {
		if check.expected != check.actual {
			return fmt.Errorf(
				"%s is %q; expected %q",
				check.field,
				check.actual,
				check.expected,
			)
		}
	}
	for _, check := range []struct {
		field    string
		expected string
		actual   string
	}{
		{"modulus", expected.Modulus, actual.Modulus},
		{
			"ecdsa_curve_name",
			expected.ECDSACurveName,
			actual.ECDSACurveName,
		},
		{"ecdsa_public", expected.ECDSAPublic, actual.ECDSAPublic},
	} {
		if check.actual != "" && check.expected != check.actual {
			return fmt.Errorf(
				"%s is %q; expected %q",
				check.field,
				check.actual,
				check.expected,
			)
		}
	}
	if expected.Created != actual.Created {
		return fmt.Errorf(
			"created is %d; expected %d",
			actual.Created,
			expected.Created,
		)
	}
	if !equalStrings(expected.Domains, actual.Domains) {
		return fmt.Errorf(
			"domains are %v; expected %v",
			actual.Domains,
			expected.Domains,
		)
	}

	return nil
}

func verifyIdentity(actual CSR, expected Identity) error {
	if actual.ID != expected.ID {
		return fmt.Errorf(
			"SSL CSR id is %q; expected %q",
			actual.ID,
			expected.ID,
		)
	}
	if actual.FingerprintSHA256 != expected.FingerprintSHA256 {
		return fmt.Errorf(
			"SSL CSR %q fingerprint is %q; expected %q",
			actual.ID,
			actual.FingerprintSHA256,
			expected.FingerprintSHA256,
		)
	}

	return nil
}

// VerifyIdentity checks that a CSR still has the expected stable identity.
func VerifyIdentity(actual CSR, expected Identity) error {
	if err := ValidateIdentity(expected); err != nil {
		return err
	}

	return verifyIdentity(actual, expected)
}

func verifyInventoryMetadata(expected, actual CSR) error {
	checks := []struct {
		field    string
		expected string
		actual   string
	}{
		{"id", expected.ID, actual.ID},
		{"friendly_name", expected.FriendlyName, actual.FriendlyName},
		{"commonName", expected.CommonName, actual.CommonName},
		{"key_algorithm", expected.KeyAlgorithm, actual.KeyAlgorithm},
		{"modulus", expected.Modulus, actual.Modulus},
		{
			"ecdsa_curve_name",
			expected.ECDSACurveName,
			actual.ECDSACurveName,
		},
		{"ecdsa_public", expected.ECDSAPublic, actual.ECDSAPublic},
	}
	for _, check := range checks {
		if check.expected != check.actual {
			return fmt.Errorf(
				"%s is %q; expected %q",
				check.field,
				check.actual,
				check.expected,
			)
		}
	}
	if expected.Created != actual.Created {
		return fmt.Errorf(
			"created is %d; expected %d",
			actual.Created,
			expected.Created,
		)
	}
	if !equalStrings(expected.Domains, actual.Domains) {
		return fmt.Errorf(
			"domains are %v; expected %v",
			actual.Domains,
			expected.Domains,
		)
	}

	return nil
}

func verifyDefinition(actual CSR, expected Definition) error {
	if actual.FriendlyName != expected.FriendlyName {
		return fmt.Errorf(
			"friendly name is %q; expected %q",
			actual.FriendlyName,
			expected.FriendlyName,
		)
	}
	if actual.CommonName != expected.Domains[0] {
		return fmt.Errorf(
			"common name is %q; expected %q",
			actual.CommonName,
			expected.Domains[0],
		)
	}
	expectedDomains := append([]string(nil), expected.Domains...)
	sort.Strings(expectedDomains)
	if !equalStrings(actual.Domains, expectedDomains) {
		return fmt.Errorf(
			"domains are %v; expected %v",
			actual.Domains,
			expectedDomains,
		)
	}

	checks := []struct {
		field    string
		expected string
		actual   string
	}{
		{"country name", expected.CountryName, actual.CountryName},
		{
			"state or province name",
			expected.StateOrProvinceName,
			actual.StateOrProvinceName,
		},
		{"locality name", expected.LocalityName, actual.LocalityName},
		{
			"organization name",
			expected.OrganizationName,
			actual.OrganizationName,
		},
		{
			"organizational unit name",
			expected.OrganizationalUnitName,
			actual.OrganizationalUnitName,
		},
		{"email address", expected.EmailAddress, actual.EmailAddress},
	}
	for _, check := range checks {
		if check.expected != check.actual {
			return fmt.Errorf(
				"%s is %q; expected %q",
				check.field,
				check.actual,
				check.expected,
			)
		}
	}

	return nil
}

// VerifyDefinition checks immutable CSR metadata against a generation request.
func VerifyDefinition(actual CSR, expected Definition) error {
	if err := ValidateDefinition(expected); err != nil {
		return err
	}

	return verifyDefinition(actual, expected)
}

func verifyCSRUnchanged(before, after CSR) error {
	if before.ID != after.ID ||
		before.FingerprintSHA256 != after.FingerprintSHA256 ||
		before.CSRPEM != after.CSRPEM ||
		before.CommonName != after.CommonName ||
		before.CountryName != after.CountryName ||
		before.StateOrProvinceName != after.StateOrProvinceName ||
		before.LocalityName != after.LocalityName ||
		before.OrganizationName != after.OrganizationName ||
		before.OrganizationalUnitName != after.OrganizationalUnitName ||
		before.EmailAddress != after.EmailAddress ||
		before.Created != after.Created ||
		!equalStrings(before.Domains, after.Domains) ||
		before.KeyAlgorithm != after.KeyAlgorithm ||
		before.Modulus != after.Modulus ||
		before.ECDSACurveName != after.ECDSACurveName ||
		before.ECDSAPublic != after.ECDSAPublic {
		return fmt.Errorf("CSR metadata or PEM changed during rename")
	}

	return nil
}
