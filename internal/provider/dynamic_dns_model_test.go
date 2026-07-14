package provider

import (
	"testing"

	"terraform-provider-cpanel/internal/cpanel/ddns"
)

func TestDynamicDNSToDataSourceModel(t *testing.T) {
	t.Parallel()

	lastUpdateTime := int64(1784046400)
	model, diagnostics := dynamicDNSToDataSourceModel(
		t.Context(),
		ddns.Domain{
			CreatedTime:    1784046318,
			Description:    "Home network",
			Domain:         "home.example.com",
			ID:             "dynamic-dns-id",
			IPv4:           []string{"192.0.2.10"},
			IPv6:           []string{"2001:db8::10"},
			LastRunTimes:   []int64{1784046500},
			LastUpdateTime: &lastUpdateTime,
		},
	)
	if diagnostics.HasError() {
		t.Fatalf("dynamicDNSToDataSourceModel() diagnostics: %v", diagnostics)
	}
	if model == nil {
		t.Fatal("dynamicDNSToDataSourceModel() returned nil")
	}
	if model.WebcallURL.ValueString() !=
		"https://home.example.com/cpanelwebcall/dynamic-dns-id" {
		t.Fatalf("webcall URL = %q", model.WebcallURL.ValueString())
	}
	if model.LastUpdateTime.ValueInt64() != lastUpdateTime {
		t.Fatalf(
			"last update time = %d",
			model.LastUpdateTime.ValueInt64(),
		)
	}
	if len(model.IPv4.Elements()) != 1 ||
		len(model.IPv6.Elements()) != 1 ||
		len(model.LastRunTimes.Elements()) != 1 {
		t.Fatalf("model collections = %#v", model)
	}
}

func TestDynamicDNSNullCollectionsAndUpdateTime(t *testing.T) {
	t.Parallel()

	model, diagnostics := dynamicDNSToDataSourceModel(
		t.Context(),
		ddns.Domain{
			Domain: "home.example.com",
			ID:     "dynamic-dns-id",
		},
	)
	if diagnostics.HasError() {
		t.Fatalf("dynamicDNSToDataSourceModel() diagnostics: %v", diagnostics)
	}
	if model.LastUpdateTime.IsNull() != true {
		t.Fatal("last update time must be null")
	}
	if len(model.IPv4.Elements()) != 0 ||
		len(model.IPv6.Elements()) != 0 ||
		len(model.LastRunTimes.Elements()) != 0 {
		t.Fatalf("model collections = %#v", model)
	}
}
