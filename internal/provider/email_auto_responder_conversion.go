package provider

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func autoResponderFromModel(model EmailAutoResponderModel) cpanelmail.AutoResponder {
	start := model.StartUnix.ValueInt64()
	stop := model.StopUnix.ValueInt64()
	isHTML := int64(0)
	if model.IsHTML.ValueBool() {
		isHTML = 1
	}

	return cpanelmail.AutoResponder{
		Email:    model.Email.ValueString(),
		From:     model.From.ValueString(),
		Subject:  model.Subject.ValueString(),
		Body:     model.Body.ValueString(),
		Charset:  model.Charset.ValueString(),
		Interval: model.IntervalHours.ValueInt64(),
		IsHTML:   isHTML,
		Start:    &start,
		Stop:     &stop,
	}
}

func applyAutoResponderToModel(
	model *EmailAutoResponderModel,
	autoResponder cpanelmail.AutoResponder,
) {
	model.Email = types.StringValue(autoResponder.Email)
	model.From = types.StringValue(autoResponder.From)
	model.Subject = types.StringValue(autoResponder.Subject)
	model.Body = types.StringValue(normalizeAutoResponderBody(autoResponder.Body))
	model.Charset = types.StringValue(autoResponder.Charset)
	model.IntervalHours = types.Int64Value(autoResponder.Interval)
	model.IsHTML = types.BoolValue(autoResponder.IsHTML == 1)
	model.StartUnix = types.Int64Value(autoResponder.StartUnix())
	model.StopUnix = types.Int64Value(autoResponder.StopUnix())
}

func normalizeAutoResponderBody(body string) string {
	body = strings.TrimSuffix(body, "\n")

	return strings.TrimSuffix(body, "\r")
}

func autoRespondersEqual(
	left cpanelmail.AutoResponder,
	right cpanelmail.AutoResponder,
) bool {
	return left.Email == right.Email &&
		left.From == right.From &&
		left.Subject == right.Subject &&
		normalizeAutoResponderBody(left.Body) == normalizeAutoResponderBody(right.Body) &&
		left.Charset == right.Charset &&
		left.Interval == right.Interval &&
		left.IsHTML == right.IsHTML &&
		left.StartUnix() == right.StartUnix() &&
		left.StopUnix() == right.StopUnix()
}
