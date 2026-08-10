package logmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) Get(ctx context.Context) (*Settings, error) {
	response := settingsResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleLogManager,
		operationGetSettings,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	archiveLogs, err := parseBooleanFlag(
		"archive_logs",
		response.Data.ArchiveLogs,
	)
	if err != nil {
		return nil, err
	}
	pruneArchive, err := parseBooleanFlag(
		"prune_archive",
		response.Data.PruneArchive,
	)
	if err != nil {
		return nil, err
	}
	usingDefault, err := parseBooleanFlag(
		"using_default",
		response.Data.UsingDefault,
	)
	if err != nil {
		return nil, err
	}
	retentionDays, err := parseRetentionDays(
		response.Data.RetentionDays,
	)
	if err != nil {
		return nil, err
	}

	return &Settings{
		ArchiveLogs:   archiveLogs,
		PruneArchive:  pruneArchive,
		RetentionDays: retentionDays,
		UsingDefault:  usingDefault,
	}, nil
}

func (c *Client) Set(
	ctx context.Context,
	definition Definition,
) (*Settings, error) {
	if err := ValidateDefinition(definition); err != nil {
		return nil, err
	}

	response := mutationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleLogManager,
		operationSetSettings,
		map[string]string{
			"archive_logs":   booleanParameter(definition.ArchiveLogs),
			"prune_archive":  booleanParameter(definition.PruneArchive),
			"retention_days": strconv.FormatInt(definition.RetentionDays, 10),
		},
		&response,
	); err != nil {
		return nil, err
	}

	actual, err := c.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("read log settings after mutation: %w", err)
	}
	if err := verifySettings(*actual, definition); err != nil {
		return nil, err
	}

	return actual, nil
}

func ValidateDefinition(definition Definition) error {
	if definition.RetentionDays < -1 {
		return fmt.Errorf(
			"log retention days must be -1, 0, or a positive integer",
		)
	}

	return nil
}

func SettingsMatchDefinition(
	settings Settings,
	definition Definition,
) bool {
	return verifySettings(settings, definition) == nil
}

func verifySettings(
	actual Settings,
	expected Definition,
) error {
	if actual.ArchiveLogs != expected.ArchiveLogs {
		return fmt.Errorf(
			"cPanel archive_logs is %t after mutation; expected %t",
			actual.ArchiveLogs,
			expected.ArchiveLogs,
		)
	}
	if actual.PruneArchive != expected.PruneArchive {
		return fmt.Errorf(
			"cPanel prune_archive is %t after mutation; expected %t",
			actual.PruneArchive,
			expected.PruneArchive,
		)
	}
	if expected.RetentionDays == -1 {
		if !actual.UsingDefault {
			return fmt.Errorf(
				"cPanel log retention is custom after mutation; expected the server default",
			)
		}

		return nil
	}
	if actual.UsingDefault {
		return fmt.Errorf(
			"cPanel log retention uses the server default after mutation; expected %d days",
			expected.RetentionDays,
		)
	}
	if actual.RetentionDays != expected.RetentionDays {
		return fmt.Errorf(
			"cPanel log retention is %d days after mutation; expected %d",
			actual.RetentionDays,
			expected.RetentionDays,
		)
	}

	return nil
}

func parseBooleanFlag(
	name string,
	raw json.RawMessage,
) (bool, error) {
	value := bytes.TrimSpace(raw)
	switch string(value) {
	case "0", `"0"`:
		return false, nil
	case "1", `"1"`:
		return true, nil
	default:
		return false, fmt.Errorf(
			"cPanel returned invalid %s flag %q; expected 0 or 1",
			name,
			value,
		)
	}
}

func parseRetentionDays(raw json.RawMessage) (int64, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 ||
		bytes.Equal(value, []byte("null")) {
		return 0, fmt.Errorf(
			"cPanel returned a missing log retention period",
		)
	}

	text := string(value)
	if value[0] == '"' {
		var decoded string
		if err := json.Unmarshal(value, &decoded); err != nil {
			return 0, fmt.Errorf(
				"decode cPanel log retention period: %w",
				err,
			)
		}
		if strings.TrimSpace(decoded) != decoded {
			return 0, fmt.Errorf(
				"cPanel returned log retention period %q with surrounding whitespace",
				decoded,
			)
		}
		text = decoded
	}

	retentionDays, err := strconv.ParseInt(text, 10, 64)
	if err != nil || retentionDays < 0 {
		return 0, fmt.Errorf(
			"cPanel returned invalid log retention period %q",
			text,
		)
	}

	return retentionDays, nil
}

func booleanParameter(value bool) string {
	if value {
		return "1"
	}

	return "0"
}
