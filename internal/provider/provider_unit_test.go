package provider

import (
	"context"
	"strings"
	"testing"

	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"terraform-provider-cpanel/internal/cpanel/cron"
	"terraform-provider-cpanel/internal/cpanel/ftp"
	"terraform-provider-cpanel/internal/cpanel/mysql"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
)

func TestValidateCronField(t *testing.T) {
	t.Parallel()

	minute := cronFieldValidator{
		name:      "minute",
		minimum:   0,
		maximum:   59,
		allowList: true,
		allowStep: true,
	}
	day := cronFieldValidator{
		name:      "day",
		minimum:   1,
		maximum:   31,
		allowStep: true,
	}
	weekday := cronFieldValidator{
		name:    "weekday",
		minimum: 0,
		maximum: 7,
	}
	month := cronFieldValidator{
		name:      "month",
		minimum:   1,
		maximum:   12,
		allowStep: true,
	}

	testCases := map[string]struct {
		field     cronFieldValidator
		value     string
		wantError bool
	}{
		"minute wildcard":        {field: minute, value: "*"},
		"minute lower bound":     {field: minute, value: "0"},
		"minute upper bound":     {field: minute, value: "59"},
		"minute step":            {field: minute, value: "*/5"},
		"minute list":            {field: minute, value: "0,30"},
		"minute above range":     {field: minute, value: "60", wantError: true},
		"minute zero step":       {field: minute, value: "*/0", wantError: true},
		"minute duplicate list":  {field: minute, value: "0,0", wantError: true},
		"minute range syntax":    {field: minute, value: "1-5", wantError: true},
		"minute with whitespace": {field: minute, value: " 5", wantError: true},
		"day lower bound":        {field: day, value: "1"},
		"day upper bound":        {field: day, value: "31"},
		"day step":               {field: day, value: "*/15"},
		"day below range":        {field: day, value: "0", wantError: true},
		"day above range":        {field: day, value: "32", wantError: true},
		"day list":               {field: day, value: "1,15", wantError: true},
		"weekday sunday zero":    {field: weekday, value: "0"},
		"weekday sunday seven":   {field: weekday, value: "7"},
		"weekday wildcard":       {field: weekday, value: "*"},
		"weekday above range":    {field: weekday, value: "8", wantError: true},
		"weekday step":           {field: weekday, value: "*/2", wantError: true},
		"month step":             {field: month, value: "*/3"},
		"month list":             {field: month, value: "1,4,7", wantError: true},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateCronField(testCase.value, testCase.field)
			if testCase.wantError && err == nil {
				t.Fatalf("validateCronField(%q) returned no error", testCase.value)
			}
			if !testCase.wantError && err != nil {
				t.Fatalf("validateCronField(%q) returned error: %v", testCase.value, err)
			}
		})
	}
}

func TestValidatePostgreSQLAccountName(t *testing.T) {
	t.Parallel()

	username := "account1234"
	testCases := map[string]struct {
		name      string
		wantError bool
	}{
		"valid":             {name: username + "_database"},
		"maximum length":    {name: username + "_" + strings.Repeat("a", 63-len(username)-1)},
		"missing prefix":    {name: "other_database", wantError: true},
		"prefix only":       {name: username + "_", wantError: true},
		"invalid character": {name: username + "_invalid-name", wantError: true},
		"too long":          {name: username + "_" + strings.Repeat("a", 64-len(username)), wantError: true},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validatePostgreSQLAccountName(username, testCase.name)
			if testCase.wantError && err == nil {
				t.Fatalf("validatePostgreSQLAccountName(%q) returned no error", testCase.name)
			}
			if !testCase.wantError && err != nil {
				t.Fatalf("validatePostgreSQLAccountName(%q) returned error: %v", testCase.name, err)
			}
		})
	}
}

func TestValidateMySQLName(t *testing.T) {
	t.Parallel()

	const (
		prefix    = "account1234_"
		maxLength = 32
	)
	testCases := map[string]struct {
		name      string
		wantError bool
	}{
		"valid":             {name: prefix + "database"},
		"maximum length":    {name: prefix + strings.Repeat("a", maxLength-len(prefix))},
		"missing prefix":    {name: "other_database", wantError: true},
		"prefix only":       {name: prefix, wantError: true},
		"invalid character": {name: prefix + "invalid-name", wantError: true},
		"too long":          {name: prefix + strings.Repeat("a", maxLength-len(prefix)+1), wantError: true},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateMySQLName(testCase.name, prefix, maxLength)
			if testCase.wantError && err == nil {
				t.Fatalf("validateMySQLName(%q) returned no error", testCase.name)
			}
			if !testCase.wantError && err != nil {
				t.Fatalf("validateMySQLName(%q) returned error: %v", testCase.name, err)
			}
		})
	}
}

func TestSplitEmailAccountAddress(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		address    string
		wantUser   string
		wantDomain string
		wantError  bool
	}{
		"valid": {
			address:    "terraform.user@example.test",
			wantUser:   "terraform.user",
			wantDomain: "example.test",
		},
		"multiple at": {
			address:   "terraform@@example.test",
			wantError: true,
		},
		"invalid user": {
			address:   "terraform+tag@example.test",
			wantError: true,
		},
		"invalid dot": {
			address:   ".terraform@example.test",
			wantError: true,
		},
		"uppercase domain": {
			address:   "terraform@Example.test",
			wantError: true,
		},
		"invalid domain label": {
			address:   "terraform@-example.test",
			wantError: true,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			user, domain, err := splitEmailAccountAddress(testCase.address)
			if testCase.wantError {
				if err == nil {
					t.Fatalf("splitEmailAccountAddress(%q) returned no error", testCase.address)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitEmailAccountAddress(%q) error: %v", testCase.address, err)
			}
			if user != testCase.wantUser || domain != testCase.wantDomain {
				t.Fatalf(
					"splitEmailAccountAddress(%q) = %q, %q; want %q, %q",
					testCase.address,
					user,
					domain,
					testCase.wantUser,
					testCase.wantDomain,
				)
			}
		})
	}
}

func TestSplitFTPAccountUsername(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		username   string
		wantUser   string
		wantDomain string
		wantError  bool
	}{
		"valid": {
			username:   "terraform.user@example.test",
			wantUser:   "terraform.user",
			wantDomain: "example.test",
		},
		"multiple at": {
			username:  "terraform@@example.test",
			wantError: true,
		},
		"invalid user": {
			username:  "terraform+tag@example.test",
			wantError: true,
		},
		"uppercase domain": {
			username:  "terraform@Example.test",
			wantError: true,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			user, domain, err := splitFTPAccountUsername(testCase.username)
			if testCase.wantError {
				if err == nil {
					t.Fatalf("splitFTPAccountUsername(%q) returned no error", testCase.username)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitFTPAccountUsername(%q) error: %v", testCase.username, err)
			}
			if user != testCase.wantUser || domain != testCase.wantDomain {
				t.Fatalf(
					"splitFTPAccountUsername(%q) = %q, %q; want %q, %q",
					testCase.username,
					user,
					domain,
					testCase.wantUser,
					testCase.wantDomain,
				)
			}
		})
	}
}

func TestValidateFTPHomeDirectory(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		homeDirectory string
		wantError     bool
	}{
		"valid":          {homeDirectory: "sites/terraform"},
		"absolute":       {homeDirectory: "/home/account/sites", wantError: true},
		"parent segment": {homeDirectory: "sites/../private", wantError: true},
		"dot segment":    {homeDirectory: "sites/./public", wantError: true},
		"empty":          {homeDirectory: "", wantError: true},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateFTPHomeDirectory(testCase.homeDirectory)
			if testCase.wantError && err == nil {
				t.Fatalf("validateFTPHomeDirectory(%q) returned no error", testCase.homeDirectory)
			}
			if !testCase.wantError && err != nil {
				t.Fatalf(
					"validateFTPHomeDirectory(%q) returned error: %v",
					testCase.homeDirectory,
					err,
				)
			}
		})
	}
}

func TestValidateDomainName(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		domain    string
		wantError bool
	}{
		"valid":         {domain: "terraform.example.test"},
		"uppercase":     {domain: "Terraform.example.test", wantError: true},
		"single label":  {domain: "localhost", wantError: true},
		"leading dash":  {domain: "-terraform.example.test", wantError: true},
		"trailing dash": {domain: "terraform-.example.test", wantError: true},
		"empty label":   {domain: "terraform..example.test", wantError: true},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateDomainName(testCase.domain)
			if testCase.wantError && err == nil {
				t.Fatalf("validateDomainName(%q) returned no error", testCase.domain)
			}
			if !testCase.wantError && err != nil {
				t.Fatalf("validateDomainName(%q) returned error: %v", testCase.domain, err)
			}
		})
	}
}

func TestValidateDomainDocumentRoot(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		documentRoot string
		wantError    bool
	}{
		"valid":          {documentRoot: "public_html/terraform"},
		"absolute":       {documentRoot: "/public_html/terraform", wantError: true},
		"parent segment": {documentRoot: "public_html/../mail", wantError: true},
		"reserved":       {documentRoot: ".ssh/terraform", wantError: true},
		"empty":          {documentRoot: "", wantError: true},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateDomainDocumentRoot(testCase.documentRoot)
			if testCase.wantError && err == nil {
				t.Fatalf(
					"validateDomainDocumentRoot(%q) returned no error",
					testCase.documentRoot,
				)
			}
			if !testCase.wantError && err != nil {
				t.Fatalf(
					"validateDomainDocumentRoot(%q) returned error: %v",
					testCase.documentRoot,
					err,
				)
			}
		})
	}
}

func TestValidateAddonDomainInternalSubdomain(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		name      string
		wantError bool
	}{
		"valid":         {name: "terraform-addon"},
		"uppercase":     {name: "Terraform-addon", wantError: true},
		"leading dash":  {name: "-terraform", wantError: true},
		"trailing dash": {name: "terraform-", wantError: true},
		"dot":           {name: "terraform.addon", wantError: true},
		"empty":         {name: "", wantError: true},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateAddonDomainInternalSubdomain(testCase.name)
			if testCase.wantError && err == nil {
				t.Fatalf(
					"validateAddonDomainInternalSubdomain(%q) returned no error",
					testCase.name,
				)
			}
			if !testCase.wantError && err != nil {
				t.Fatalf(
					"validateAddonDomainInternalSubdomain(%q) returned error: %v",
					testCase.name,
					err,
				)
			}
		})
	}
}

func TestCronJobModelLookupsIgnoreVariables(t *testing.T) {
	t.Parallel()

	details := cron.CronJobDetailsModel{
		Command: "/bin/true",
		Minute:  "0",
		Hour:    "1",
		Day:     "*",
		Weekday: "*",
		Month:   "*",
	}
	data := &cron.CronJobDataSourceModel{
		CpanelResult: cron.CronJobCpanelResultModel{
			Data: []cron.CronJobDataSourceDataModel{
				{
					CronJobDetailsModel: details,
					LineKey:             "variable-line-key",
					Type:                "variable",
				},
				{
					CronJobDetailsModel: details,
					LineKey:             "command-line-key",
					Type:                "command",
				},
				{
					CronJobDetailsModel: details,
					LineKey:             "second-command-line-key",
					Type:                "command",
				},
			},
		},
	}

	model := CronJobAPIToModelByLineKey(data, "command-line-key")
	if model == nil {
		t.Fatal("CronJobAPIToModelByLineKey returned nil")
	}
	if got := model.Command.ValueString(); got != details.Command {
		t.Fatalf("command = %q, want %q", got, details.Command)
	}

	internalID := CalculateCronJobDataSourceDataModelInternalID(
		cron.CronJobDataSourceDataModel{CronJobDetailsModel: details},
	)
	matches := CronJobAPIToModelsByInternalID(data, internalID)
	if len(matches) != 2 {
		t.Fatalf("matching command count = %d, want 2", len(matches))
	}
}

func TestPostgreSQLDatabaseAPIToModelUsesSet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	data := &postgresql.DatabaseDataSourceModel{
		Data: []postgresql.DatabaseDataSourceDataModel{
			{
				Database: "account1234_database",
				Users: []string{
					"account1234_second",
					"account1234_first",
				},
			},
		},
	}

	model, diagnostics := PostgreSQLDatabaseAPIToModel(ctx, data, "account1234_database")
	if diagnostics.HasError() {
		t.Fatalf("PostgreSQLDatabaseAPIToModel returned diagnostics: %v", diagnostics)
	}
	if model == nil {
		t.Fatal("PostgreSQLDatabaseAPIToModel returned nil")
	}

	var users []string
	diagnostics = model.Users.ElementsAs(ctx, &users, false)
	if diagnostics.HasError() {
		t.Fatalf("read users set: %v", diagnostics)
	}
	if len(users) != 2 {
		t.Fatalf("users count = %d, want 2", len(users))
	}

	data.Data[0].Users = nil
	model, diagnostics = PostgreSQLDatabaseAPIToModel(
		ctx,
		data,
		"account1234_database",
	)
	if diagnostics.HasError() {
		t.Fatalf("nil users returned diagnostics: %v", diagnostics)
	}
	if model.Users.IsNull() || model.Users.IsUnknown() ||
		len(model.Users.Elements()) != 0 {
		t.Fatalf("nil users produced %#v, want a known empty set", model.Users)
	}

	missing, diagnostics := PostgreSQLDatabaseAPIToModel(ctx, data, "account1234_missing")
	if diagnostics.HasError() {
		t.Fatalf("missing database returned diagnostics: %v", diagnostics)
	}
	if missing != nil {
		t.Fatalf("missing database model = %#v, want nil", missing)
	}
}

func TestMySQLDatabaseAPIToModelUsesSet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	data := &mysql.DatabaseListResponse{
		Data: []mysql.Database{
			{
				Database: "account1234_database",
				Users: []string{
					"account1234_second",
					"account1234_first",
				},
			},
		},
	}

	model, diagnostics := MySQLDatabaseAPIToModel(ctx, data, "account1234_database")
	if diagnostics.HasError() {
		t.Fatalf("MySQLDatabaseAPIToModel returned diagnostics: %v", diagnostics)
	}
	if model == nil {
		t.Fatal("MySQLDatabaseAPIToModel returned nil")
	}

	var users []string
	diagnostics = model.Users.ElementsAs(ctx, &users, false)
	if diagnostics.HasError() {
		t.Fatalf("read users set: %v", diagnostics)
	}
	if len(users) != 2 {
		t.Fatalf("users count = %d, want 2", len(users))
	}

	data.Data[0].Users = nil
	model, diagnostics = MySQLDatabaseAPIToModel(
		ctx,
		data,
		"account1234_database",
	)
	if diagnostics.HasError() {
		t.Fatalf("nil users returned diagnostics: %v", diagnostics)
	}
	if model.Users.IsNull() || model.Users.IsUnknown() ||
		len(model.Users.Elements()) != 0 {
		t.Fatalf("nil users produced %#v, want a known empty set", model.Users)
	}

	missing, diagnostics := MySQLDatabaseAPIToModel(ctx, data, "account1234_missing")
	if diagnostics.HasError() {
		t.Fatalf("missing database returned diagnostics: %v", diagnostics)
	}
	if missing != nil {
		t.Fatalf("missing database model = %#v, want nil", missing)
	}
}

func TestStringSetDifference(t *testing.T) {
	t.Parallel()

	got := stringSetDifference(
		[]string{"second", "first", "third"},
		[]string{"second"},
	)
	want := []string{"first", "third"}
	if len(got) != len(want) {
		t.Fatalf("difference = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("difference = %v, want %v", got, want)
		}
	}
}

func TestProviderSchemaProtectsAPIToken(t *testing.T) {
	t.Parallel()

	var response frameworkprovider.SchemaResponse
	(&cpanelProvider{}).Schema(
		context.Background(),
		frameworkprovider.SchemaRequest{},
		&response,
	)

	apiToken, ok := response.Schema.Attributes["api_token"].(providerschema.StringAttribute)
	if !ok {
		t.Fatalf("api_token has type %T, want schema.StringAttribute", response.Schema.Attributes["api_token"])
	}
	if !apiToken.Sensitive {
		t.Fatal("api_token is not marked sensitive")
	}

	apiTokenName, ok := response.Schema.Attributes["api_token_name"].(providerschema.StringAttribute)
	if !ok {
		t.Fatalf(
			"api_token_name has type %T, want schema.StringAttribute",
			response.Schema.Attributes["api_token_name"],
		)
	}
	if !apiTokenName.Optional || apiTokenName.Sensitive {
		t.Fatal("api_token_name must be optional and non-sensitive")
	}
}

func TestPostgreSQLUserDataSourceSchemaDoesNotExposePassword(t *testing.T) {
	t.Parallel()

	var response frameworkdatasource.SchemaResponse
	(&postgreSQLUserDataSource{}).Schema(
		context.Background(),
		frameworkdatasource.SchemaRequest{},
		&response,
	)

	for _, removedAttribute := range []string{"password", "last_updated"} {
		if _, exists := response.Schema.Attributes[removedAttribute]; exists {
			t.Fatalf("data source schema still exposes %q", removedAttribute)
		}
	}
}

func TestMySQLUserDataSourceSchemaDoesNotExposePassword(t *testing.T) {
	t.Parallel()

	var response frameworkdatasource.SchemaResponse
	(&mySQLUserDataSource{}).Schema(
		context.Background(),
		frameworkdatasource.SchemaRequest{},
		&response,
	)

	if _, exists := response.Schema.Attributes["password"]; exists {
		t.Fatal("MySQL user data source schema exposes password")
	}
}

func TestEmailAccountDataSourceSchemaDoesNotExposePassword(t *testing.T) {
	t.Parallel()

	var response frameworkdatasource.SchemaResponse
	(&emailAccountDataSource{}).Schema(
		context.Background(),
		frameworkdatasource.SchemaRequest{},
		&response,
	)

	if _, exists := response.Schema.Attributes["password"]; exists {
		t.Fatal("email account data source schema exposes password")
	}
}

func TestFTPAccountDataSourceSchemaDoesNotExposePassword(t *testing.T) {
	t.Parallel()

	var response frameworkdatasource.SchemaResponse
	(&ftpAccountDataSource{}).Schema(
		context.Background(),
		frameworkdatasource.SchemaRequest{},
		&response,
	)

	if _, exists := response.Schema.Attributes["password"]; exists {
		t.Fatal("FTP account data source schema exposes password")
	}
}

func TestPasswordResourcesUseWriteOnlyVersionedSecrets(t *testing.T) {
	t.Parallel()

	resources := map[string]frameworkresource.Resource{
		"Directory Privacy user": NewDirectoryPrivacyUserResource(),
		"email account":          NewEmailAccountResource(),
		"FTP account":            NewFTPAccountResource(),
		"MySQL user":             NewMySQLUserResource(),
		"PostgreSQL user":        NewPostgreSQLUserResource(),
	}
	for name, resourceUnderTest := range resources {
		name := name
		resourceUnderTest := resourceUnderTest
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var response frameworkresource.SchemaResponse
			resourceUnderTest.Schema(
				t.Context(),
				frameworkresource.SchemaRequest{},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
			}

			password, ok := response.Schema.Attributes["password"].(resourceschema.StringAttribute)
			if !ok ||
				!password.Optional ||
				!password.Sensitive ||
				!password.WriteOnly {
				t.Fatal("password must be optional, sensitive, and write-only")
			}
			passwordVersion, ok := response.Schema.Attributes["password_version"].(resourceschema.Int64Attribute)
			if !ok || !passwordVersion.Optional {
				t.Fatal("password_version must be an optional integer")
			}
			deleteOnDestroy, ok := response.Schema.Attributes["delete_on_destroy"].(resourceschema.BoolAttribute)
			if !ok ||
				!deleteOnDestroy.Optional ||
				!deleteOnDestroy.Computed ||
				deleteOnDestroy.Default == nil {
				t.Fatal("delete_on_destroy must be optional, computed, and defaulted")
			}
		})
	}
}

func TestFTPAccountDefaultsToPreservingHomeDirectory(t *testing.T) {
	t.Parallel()

	var response frameworkresource.SchemaResponse
	(&ftpAccountResource{}).Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&response,
	)

	attribute, ok := response.Schema.Attributes["delete_home_directory"].(resourceschema.BoolAttribute)
	if !ok {
		t.Fatalf(
			"delete_home_directory has type %T, want schema.BoolAttribute",
			response.Schema.Attributes["delete_home_directory"],
		)
	}
	if attribute.Default == nil {
		t.Fatal("delete_home_directory has no default")
	}
	deleteOnDestroy, ok := response.Schema.Attributes["delete_on_destroy"].(resourceschema.BoolAttribute)
	if !ok {
		t.Fatalf(
			"delete_on_destroy has type %T, want schema.BoolAttribute",
			response.Schema.Attributes["delete_on_destroy"],
		)
	}
	if deleteOnDestroy.Default == nil {
		t.Fatal("delete_on_destroy has no default")
	}
}

func TestSubdomainDefaultsToPreservingDocumentRoot(t *testing.T) {
	t.Parallel()

	var response frameworkresource.SchemaResponse
	(&subdomainResource{}).Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&response,
	)

	attribute, ok := response.Schema.Attributes["delete_document_root"].(resourceschema.BoolAttribute)
	if !ok {
		t.Fatalf(
			"delete_document_root has type %T, want schema.BoolAttribute",
			response.Schema.Attributes["delete_document_root"],
		)
	}
	if attribute.Default == nil {
		t.Fatal("delete_document_root has no default")
	}
}

func TestAddonDomainDefaultsToPreservingDocumentRoot(t *testing.T) {
	t.Parallel()

	var response frameworkresource.SchemaResponse
	(&addonDomainResource{}).Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&response,
	)

	attribute, ok := response.Schema.Attributes["delete_document_root"].(resourceschema.BoolAttribute)
	if !ok {
		t.Fatalf(
			"delete_document_root has type %T, want schema.BoolAttribute",
			response.Schema.Attributes["delete_document_root"],
		)
	}
	if attribute.Default == nil {
		t.Fatal("delete_document_root has no default")
	}
}

func TestFTPAccountModelParsesWholeQuota(t *testing.T) {
	t.Parallel()

	account := ftp.Account{DiskQuotaRaw: []byte(`"50.50"`)}
	if _, err := account.QuotaMiB(); err == nil {
		t.Fatal("FTP quota with a fractional MiB returned no error")
	}
}

func TestPostgreSQLDatabaseUsersAreASet(t *testing.T) {
	t.Parallel()

	var response frameworkresource.SchemaResponse
	(&postgreSQLDatabaseResource{}).Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&response,
	)

	if _, ok := response.Schema.Attributes["users"].(resourceschema.SetAttribute); !ok {
		t.Fatalf("users has type %T, want schema.SetAttribute", response.Schema.Attributes["users"])
	}
	deleteOnDestroy, ok := response.Schema.Attributes["delete_on_destroy"].(resourceschema.BoolAttribute)
	if !ok || deleteOnDestroy.Default == nil {
		t.Fatal("delete_on_destroy must be a defaulted boolean")
	}
}

func TestMySQLDatabaseUsersAreASet(t *testing.T) {
	t.Parallel()

	var response frameworkresource.SchemaResponse
	(&mySQLDatabaseResource{}).Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&response,
	)

	if _, ok := response.Schema.Attributes["users"].(resourceschema.SetAttribute); !ok {
		t.Fatalf("users has type %T, want schema.SetAttribute", response.Schema.Attributes["users"])
	}
	deleteOnDestroy, ok := response.Schema.Attributes["delete_on_destroy"].(resourceschema.BoolAttribute)
	if !ok || deleteOnDestroy.Default == nil {
		t.Fatal("delete_on_destroy must be a defaulted boolean")
	}
}

func TestCronJobResourceSupportsImport(t *testing.T) {
	t.Parallel()

	if _, ok := NewCronJobResource().(frameworkresource.ResourceWithImportState); !ok {
		t.Fatal("cron job resource does not implement ResourceWithImportState")
	}
}
