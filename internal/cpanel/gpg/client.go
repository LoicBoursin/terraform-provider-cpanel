package gpg

import (
	"context"
	"encoding/json"
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
	recoveryReadAttempts = 12
	recoveryReadDelay    = 250 * time.Millisecond
	recoveryTimeout      = 5 * time.Second
)

var mutationMu sync.Mutex

// Client manages public-only OpenPGP keys stored by cPanel.
type Client struct {
	*cpanel.Client
}

// NewClient creates a GPG client from the shared cPanel client.
func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

// ListPublic returns the complete public-key inventory sorted by full key ID.
func (c *Client) ListPublic(
	ctx context.Context,
) ([]Metadata, []string, error) {
	response := listResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleGPG,
		operationListPublicKeys,
		map[string]string{},
		&response,
	); err != nil {
		return nil, response.Warnings, err
	}
	values, err := parseInventory(response.Data, "pub", 16)

	return values, response.Warnings, err
}

// ListSecret returns metadata only for the complete secret-key inventory.
func (c *Client) ListSecret(
	ctx context.Context,
) ([]Metadata, []string, error) {
	response := listResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleGPG,
		operationListSecretKeys,
		map[string]string{},
		&response,
	); err != nil {
		return nil, response.Warnings, err
	}

	values, err := parseInventoryFlexibleSecretIDs(response.Data)

	return values, response.Warnings, err
}

// Inventory reads both inventories so callers can enforce public-only safety.
func (c *Client) Inventory(
	ctx context.Context,
) (Inventory, []string, error) {
	public, publicWarnings, err := c.ListPublic(ctx)
	if err != nil {
		return Inventory{}, publicWarnings, fmt.Errorf(
			"list cPanel GPG public keys: %w",
			err,
		)
	}
	secret, secretWarnings, err := c.ListSecret(ctx)
	warnings := mergeWarnings(publicWarnings, secretWarnings)
	if err != nil {
		return Inventory{}, warnings, fmt.Errorf(
			"list cPanel GPG secret keys: %w",
			err,
		)
	}

	return Inventory{Public: public, Secret: secret}, warnings, nil
}

// Lookup returns one exported public key plus the matching secret-key signal.
func (c *Client) Lookup(
	ctx context.Context,
	id string,
) (*Lookup, []string, error) {
	if err := ValidatePublicID(id); err != nil {
		return nil, nil, err
	}

	inventory, inventoryWarnings, err := c.Inventory(ctx)
	if err != nil {
		return nil, inventoryWarnings, err
	}

	return c.lookupFromInventory(ctx, id, inventory, inventoryWarnings)
}

func (c *Client) lookupFromInventory(
	ctx context.Context,
	id string,
	inventory Inventory,
	warnings []string,
) (*Lookup, []string, error) {
	hasSecret := inventoryHasSecret(inventory, id)
	index := sort.Search(len(inventory.Public), func(index int) bool {
		return inventory.Public[index].ID >= id
	})
	if index == len(inventory.Public) || inventory.Public[index].ID != id {
		return &Lookup{HasSecretKey: hasSecret}, warnings, nil
	}

	armored, exportWarnings, err := c.exportPublic(ctx, id)
	warnings = mergeWarnings(warnings, exportWarnings)
	if err != nil {
		return nil, warnings, fmt.Errorf(
			"export cPanel GPG public key %q: %w",
			id,
			err,
		)
	}
	parsed, err := ParsePublicKey(armored)
	if err != nil {
		return nil, warnings, fmt.Errorf(
			"validate exported cPanel GPG public key %q: %w",
			id,
			err,
		)
	}
	if parsed.ID != id {
		return nil, warnings, fmt.Errorf(
			"cPanel exported GPG public key id %q for requested id %q",
			parsed.ID,
			id,
		)
	}
	metadata := inventory.Public[index]
	if metadata.Bits != parsed.Bits {
		return nil, warnings, fmt.Errorf(
			"cPanel GPG public key %q inventory reports %d bits but export contains %d bits",
			id,
			metadata.Bits,
			parsed.Bits,
		)
	}

	return &Lookup{
		Key: &PublicKey{
			Metadata:      metadata,
			Armored:       parsed.Armored,
			Fingerprint:   parsed.Fingerprint,
			ContentSHA256: parsed.ContentSHA256,
		},
		HasSecretKey: hasSecret,
	}, warnings, nil
}

// Import stores one validated public-only key and proves its exact identity.
func (c *Client) Import(
	ctx context.Context,
	publicKey string,
) (*PublicKey, []string, error) {
	parsed, err := ParsePublicKey(publicKey)
	if err != nil {
		return nil, nil, err
	}

	mutationMu.Lock()
	defer mutationMu.Unlock()

	baseline, baselineWarnings, err := c.Inventory(ctx)
	if err != nil {
		return nil, baselineWarnings, err
	}
	if inventoryHasPublic(baseline, parsed.ID) {
		return nil, baselineWarnings, fmt.Errorf(
			"cPanel GPG public key %q already exists; import it into Terraform state instead",
			parsed.ID,
		)
	}
	if inventoryHasSecret(baseline, parsed.ID) {
		return nil, baselineWarnings, fmt.Errorf(
			"refusing to import GPG public key %q because cPanel already stores a matching secret key",
			parsed.ID,
		)
	}

	response := importResponse{}
	requestErr := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleGPG,
		operationImportKey,
		map[string]string{"key_data": parsed.Armored},
		&response,
	)
	warnings := mergeWarnings(baselineWarnings, response.Warnings)
	if requestErr != nil {
		if isDeterministicMutationError(requestErr) {
			return nil, warnings, requestErr
		}

		recovered, recoveryWarnings, recoveryErr :=
			c.recoverImportedAfterMutation(ctx, parsed)
		warnings = mergeWarnings(warnings, recoveryWarnings)
		if recoveryErr == nil && recovered != nil {
			warnings = mergeWarnings(
				warnings,
				[]string{
					"cPanel returned an ambiguous GPG import response; Terraform recovered the imported public key through read-only identity verification.",
				},
			)

			return recovered, warnings, nil
		}

		return nil, warnings, fmt.Errorf(
			"import cPanel GPG public key %q: %w; read-only recovery failed: %v",
			parsed.ID,
			requestErr,
			recoveryErr,
		)
	}

	returnedID, err := parseRequiredString(response.Data.KeyID, "key_id")
	if err != nil || strings.ToUpper(returnedID) != parsed.ID {
		recovered, recoveryWarnings, recoveryErr :=
			c.recoverImportedAfterMutation(ctx, parsed)
		warnings = mergeWarnings(warnings, recoveryWarnings)
		if recoveryErr == nil && recovered != nil {
			warnings = mergeWarnings(
				warnings,
				[]string{
					"cPanel returned an unexpected GPG import identifier; Terraform recovered the imported public key through read-only identity verification.",
				},
			)

			return recovered, warnings, nil
		}
		if err != nil {
			return nil, warnings, &MutationVerificationError{
				Operation: "import",
				Err: fmt.Errorf(
					"decode imported key id: %w; read-only recovery failed: %v",
					err,
					recoveryErr,
				),
			}
		}

		return nil, warnings, &MutationVerificationError{
			Operation: "import",
			Err: fmt.Errorf(
				"cPanel returned key id %q; expected %q; read-only recovery failed: %v",
				returnedID,
				parsed.ID,
				recoveryErr,
			),
		}
	}

	key, recoveryWarnings, recoveryErr :=
		c.recoverImportedAfterMutation(ctx, parsed)
	warnings = mergeWarnings(warnings, recoveryWarnings)
	if recoveryErr != nil {
		return nil, warnings, &MutationVerificationError{
			Operation: "import",
			Err: fmt.Errorf(
				"cPanel reported imported key id %q, but Terraform could not verify the canonical public packet set: %w; import that ID after resolving the read error",
				parsed.ID,
				recoveryErr,
			),
		}
	}

	return key, warnings, nil
}

// DeleteKeyPairGuarded invokes cPanel's pair-deletion API only after immutable
// public ownership checks and repeated secret-inventory checks. The Terraform
// resource deliberately does not call this method because cPanel has no
// public-only deletion operation.
func (c *Client) DeleteKeyPairGuarded(
	ctx context.Context,
	ownership Ownership,
) ([]string, error) {
	if err := ValidatePublicID(ownership.ID); err != nil {
		return nil, err
	}
	if err := validateFingerprint(ownership.Fingerprint); err != nil {
		return nil, err
	}
	if err := validateContentSHA256(ownership.ContentSHA256); err != nil {
		return nil, err
	}

	mutationMu.Lock()
	defer mutationMu.Unlock()

	lookup, warnings, err := c.Lookup(ctx, ownership.ID)
	if err != nil {
		return warnings, err
	}
	if lookup.HasSecretKey {
		return warnings, fmt.Errorf(
			"refusing to delete GPG public key %q because cPanel stores a matching secret key",
			ownership.ID,
		)
	}
	if lookup.Key == nil {
		return warnings, nil
	}
	if err := verifyOwnership(*lookup.Key, ownership); err != nil {
		return warnings, err
	}
	secret, secretWarnings, err := c.ListSecret(ctx)
	warnings = mergeWarnings(warnings, secretWarnings)
	if err != nil {
		return warnings, fmt.Errorf(
			"re-read cPanel GPG secret keys before pair deletion: %w",
			err,
		)
	}
	if inventoryHasSecret(
		Inventory{Secret: secret},
		ownership.ID,
	) {
		return warnings, fmt.Errorf(
			"refusing to delete GPG key pair %q because cPanel stores a matching secret key",
			ownership.ID,
		)
	}

	response := mutationResponse{}
	requestErr := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleGPG,
		operationDeleteKeyPair,
		map[string]string{"key_id": ownership.ID},
		&response,
	)
	warnings = mergeWarnings(warnings, response.Warnings)
	if requestErr != nil && isDeterministicMutationError(requestErr) {
		return warnings, requestErr
	}

	remaining, remainingWarnings, verifyErr := c.Lookup(ctx, ownership.ID)
	warnings = mergeWarnings(warnings, remainingWarnings)
	if verifyErr != nil {
		if requestErr != nil {
			return warnings, fmt.Errorf(
				"delete cPanel GPG public key %q: %w; verify deletion: %v",
				ownership.ID,
				requestErr,
				verifyErr,
			)
		}

		return warnings, &MutationVerificationError{
			Operation: "deletion",
			Err:       verifyErr,
		}
	}
	if remaining.HasSecretKey {
		return warnings, &MutationVerificationError{
			Operation: "deletion",
			Err: fmt.Errorf(
				"cPanel stores a matching secret key for %q after deletion",
				ownership.ID,
			),
		}
	}
	if remaining.Key == nil {
		if requestErr != nil {
			warnings = mergeWarnings(
				warnings,
				[]string{
					"cPanel returned an ambiguous GPG deletion response; Terraform recovered successful deletion through read-only inventory verification.",
				},
			)
		}

		return warnings, nil
	}
	if ownershipErr := verifyOwnership(
		*remaining.Key,
		ownership,
	); ownershipErr != nil {
		return warnings, &MutationVerificationError{
			Operation: "deletion",
			Err: fmt.Errorf(
				"GPG public key %q changed during deletion verification: %w",
				ownership.ID,
				ownershipErr,
			),
		}
	}
	if requestErr != nil {
		return warnings, requestErr
	}

	return warnings, &MutationVerificationError{
		Operation: "deletion",
		Err: fmt.Errorf(
			"GPG public key %q still exists after deletion",
			ownership.ID,
		),
	}
}

func (c *Client) recoverImportedAfterMutation(
	ctx context.Context,
	parsed *ParsedPublicKey,
) (*PublicKey, []string, error) {
	recoveryContext, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		recoveryTimeout,
	)
	defer cancel()

	return c.recoverImported(recoveryContext, parsed)
}

func (c *Client) exportPublic(
	ctx context.Context,
	id string,
) (string, []string, error) {
	response := exportResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleGPG,
		operationExportPublicKey,
		map[string]string{"key_id": id},
		&response,
	); err != nil {
		return "", response.Warnings, err
	}
	keyData, err := parseRequiredString(response.Data.KeyData, "key_data")
	if err != nil {
		return "", response.Warnings, err
	}

	return keyData, response.Warnings, nil
}

func (c *Client) recoverImported(
	ctx context.Context,
	parsed *ParsedPublicKey,
) (*PublicKey, []string, error) {
	var warnings []string
	var lastErr error
	for attempt := 0; attempt < recoveryReadAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, warnings, ctx.Err()
			case <-time.After(recoveryReadDelay):
			}
		}

		lookup, lookupWarnings, err := c.Lookup(ctx, parsed.ID)
		warnings = mergeWarnings(warnings, lookupWarnings)
		if err != nil {
			lastErr = err
			continue
		}
		key, verifyErr := verifyImportedLookup(lookup, parsed)
		if verifyErr == nil {
			return key, warnings, nil
		}
		lastErr = verifyErr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf(
			"GPG public key %q was not found",
			parsed.ID,
		)
	}

	return nil, warnings, lastErr
}

func verifyImportedLookup(
	lookup *Lookup,
	parsed *ParsedPublicKey,
) (*PublicKey, error) {
	if lookup == nil {
		return nil, fmt.Errorf("cPanel returned no GPG lookup result")
	}
	if lookup.HasSecretKey {
		return nil, fmt.Errorf(
			"cPanel stores a matching secret key for imported public key %q",
			parsed.ID,
		)
	}
	if lookup.Key == nil {
		return nil, fmt.Errorf(
			"cPanel GPG public key %q was not found",
			parsed.ID,
		)
	}
	if lookup.Key.ID != parsed.ID ||
		lookup.Key.Fingerprint != parsed.Fingerprint ||
		lookup.Key.ContentSHA256 != parsed.ContentSHA256 {
		return nil, fmt.Errorf(
			"imported GPG canonical public packet set does not match the configured key",
		)
	}

	return lookup.Key, nil
}

func verifyOwnership(key PublicKey, ownership Ownership) error {
	if key.ID != ownership.ID {
		return fmt.Errorf(
			"GPG public key id is %q; expected %q",
			key.ID,
			ownership.ID,
		)
	}
	if key.Fingerprint != ownership.Fingerprint {
		return fmt.Errorf(
			"GPG public key fingerprint is %q; expected %q",
			key.Fingerprint,
			ownership.Fingerprint,
		)
	}
	if key.ContentSHA256 != ownership.ContentSHA256 {
		return fmt.Errorf(
			"GPG public key export SHA-256 is %q; expected %q",
			key.ContentSHA256,
			ownership.ContentSHA256,
		)
	}

	return nil
}

func inventoryHasPublic(inventory Inventory, id string) bool {
	index := sort.Search(len(inventory.Public), func(index int) bool {
		return inventory.Public[index].ID >= id
	})

	return index < len(inventory.Public) && inventory.Public[index].ID == id
}

func inventoryHasSecret(inventory Inventory, publicID string) bool {
	for _, secret := range inventory.Secret {
		if secretMatchesPublicID(secret.ID, publicID) {
			return true
		}
	}

	return false
}

func parseInventoryFlexibleSecretIDs(
	raw []byte,
) ([]Metadata, error) {
	var values []apiMetadata
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("cPanel GPG secret inventory data must be an array")
	}
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode cPanel GPG secret inventory: %w", err)
	}

	result := make([]Metadata, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		id, err := parseRequiredString(value.ID, "id")
		if err != nil {
			return nil, fmt.Errorf(
				"decode cPanel GPG secret inventory item %d: %w",
				index,
				err,
			)
		}
		id = strings.ToUpper(id)
		if err := validateSecretID(id); err != nil {
			return nil, fmt.Errorf(
				"decode cPanel GPG secret inventory item %d: %w",
				index,
				err,
			)
		}

		metadata, err := metadataFromAPI(
			value,
			"sec",
			len(id),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"decode cPanel GPG secret inventory item %d: %w",
				index,
				err,
			)
		}
		if _, exists := seen[metadata.ID]; exists {
			return nil, fmt.Errorf(
				"cPanel returned duplicate GPG secret key id %q",
				metadata.ID,
			)
		}
		seen[metadata.ID] = struct{}{}
		result = append(result, metadata)
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].ID < result[right].ID
	})

	return result, nil
}

func isDeterministicMutationError(err error) bool {
	var apiError *cpanel.APIError

	return errors.As(err, &apiError)
}
