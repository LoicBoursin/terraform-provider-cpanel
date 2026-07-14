package provider

import (
	"testing"

	cpanelredirect "terraform-provider-cpanel/internal/cpanel/redirect"
)

func TestRedirectToDataSourceModel(t *testing.T) {
	t.Parallel()

	model, diagnostics := redirectToDataSourceModel(
		cpanelredirect.Redirect{
			Destination:  "https://example.net/new",
			DocumentRoot: "/home/account/public_html",
			Domain:       "example.com",
			Kind:         "rewrite",
			MatchWWW:     1,
			Source:       "/old",
			StatusCode:   "301",
			Type:         cpanelredirect.TypePermanent,
			Wildcard:     1,
		},
	)
	if diagnostics.HasError() {
		t.Fatalf("redirectToDataSourceModel() diagnostics: %v", diagnostics)
	}
	if model == nil {
		t.Fatal("redirectToDataSourceModel() returned nil")
	}
	if model.WWWMode.ValueString() != cpanelredirect.WWWModeBoth ||
		model.StatusCode.ValueInt64() != 301 ||
		!model.Wildcard.ValueBool() {
		t.Fatalf("model = %#v", model)
	}
}

func TestRedirectToDataSourceModelRejectsInvalidStatusCode(t *testing.T) {
	t.Parallel()

	model, diagnostics := redirectToDataSourceModel(
		cpanelredirect.Redirect{
			Domain:     "example.com",
			Source:     "/old",
			StatusCode: "invalid",
		},
	)
	if !diagnostics.HasError() {
		t.Fatal("redirectToDataSourceModel() returned no diagnostics")
	}
	if model != nil {
		t.Fatalf("model = %#v, want nil", model)
	}
}
