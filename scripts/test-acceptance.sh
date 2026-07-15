#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
env_file="${CPANEL_ENV_FILE:-${HOME}/.config/terraform-provider-cpanel/acceptance.env}"

if [[ -f "${env_file}" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${env_file}"
  set +a
fi

"${script_directory}/cpanel-smoke.sh"

if [[ -z "${CPANEL_EXPECTED_LOCALE:-}" ]]; then
  locale_response="$(
    curl \
      --silent \
      --show-error \
      --fail \
      --max-time 90 \
      --header \
      "Authorization: cpanel ${CPANEL_USERNAME}:${CPANEL_API_TOKEN}" \
      "${CPANEL_HOST%/}/execute/Locale/get_attributes"
  )"
  CPANEL_EXPECTED_LOCALE="$(
    jq -r \
      'select(.status == 1) | .data.locale // empty' \
      <<<"${locale_response}"
  )"
  if [[ -z "${CPANEL_EXPECTED_LOCALE}" ]]; then
    printf 'Unable to capture the initial cPanel locale\n' >&2
    exit 1
  fi
  export CPANEL_EXPECTED_LOCALE
fi

"${script_directory}/cpanel-clean-test-artifacts.sh"
CPANEL_REQUIRE_EMPTY=1 "${script_directory}/cpanel-smoke.sh"

cd "${repository_directory}"

export CGO_ENABLED="${CGO_ENABLED:-0}"
export TF_ACC=1

set +e
go test ./internal/provider -v -count=1 -timeout 120m "$@"
test_status=$?

"${script_directory}/cpanel-clean-test-artifacts.sh"
cleanup_status=$?

CPANEL_REQUIRE_EMPTY=1 "${script_directory}/cpanel-smoke.sh"
inventory_status=$?
set -e

if [[ "${test_status}" != "0" ]]; then
  exit "${test_status}"
fi
if [[ "${cleanup_status}" != "0" ]]; then
  exit "${cleanup_status}"
fi
exit "${inventory_status}"
