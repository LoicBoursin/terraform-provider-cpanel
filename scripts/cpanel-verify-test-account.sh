#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
env_file="${CPANEL_ENV_FILE:-${HOME}/.config/terraform-provider-cpanel/acceptance.env}"
marker_file=".terraform-provider-cpanel-acceptance-account"

source "${script_directory}/cpanel-common.sh"

if [[ -f "${env_file}" ]]; then
  cpanel_load_environment_file \
    "${env_file}" \
    'Acceptance environment file' \
    CPANEL_HOST \
    CPANEL_USERNAME \
    CPANEL_API_TOKEN \
    CPANEL_API_TOKEN_NAME \
    CPANEL_EXPECTED_TEST_HOST \
    CPANEL_EXPECTED_TEST_USERNAME \
    CPANEL_EXPECTED_VERSION \
    CPANEL_ACCEPT_DESTRUCTIVE \
    CPANEL_TEST_SSL_KEY_ID
fi

for variable in \
  CPANEL_HOST \
  CPANEL_USERNAME \
  CPANEL_API_TOKEN \
  CPANEL_API_TOKEN_NAME \
  CPANEL_EXPECTED_TEST_HOST \
  CPANEL_EXPECTED_TEST_USERNAME \
  CPANEL_EXPECTED_VERSION; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'Missing required environment variable: %s\n' "${variable}" >&2
    exit 1
  fi
done

cpanel_validate_host "${CPANEL_HOST}"
cpanel_validate_host "${CPANEL_EXPECTED_TEST_HOST}"

if [[ "${CPANEL_ACCEPT_DESTRUCTIVE:-}" != "1" ]]; then
  printf '%s\n' \
    'Refusing destructive cPanel operations: CPANEL_ACCEPT_DESTRUCTIVE must be set to 1 for a dedicated test account.' >&2
  exit 1
fi

actual_host="${CPANEL_HOST%/}"
expected_host="${CPANEL_EXPECTED_TEST_HOST%/}"
if [[ "${actual_host}" != "${expected_host}" ]]; then
  printf '%s\n' \
    'Refusing destructive cPanel operations: CPANEL_HOST does not match CPANEL_EXPECTED_TEST_HOST.' >&2
  exit 1
fi

if [[ "${CPANEL_USERNAME}" != "${CPANEL_EXPECTED_TEST_USERNAME}" ]]; then
  printf '%s\n' \
    'Refusing destructive cPanel operations: CPANEL_USERNAME does not match CPANEL_EXPECTED_TEST_USERNAME.' >&2
  exit 1
fi

if [[ "${CPANEL_API_TOKEN_NAME}" == tfcpaneltoken* ]]; then
  printf '%s\n' \
    'Refusing destructive cPanel operations: the active API token uses the reserved acceptance-test prefix.' >&2
  exit 1
fi

for command in curl jq; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "${command}" >&2
    exit 1
  fi
done

host="${CPANEL_HOST%/}"
expected_marker="$(
  cpanel_acceptance_account_marker \
    "${host}" \
    "${CPANEL_USERNAME}" \
    "${CPANEL_API_TOKEN}"
)"
response_file="$(mktemp)"
cleanup() {
  rm -f "${response_file}"
}
trap cleanup EXIT

request() {
  local endpoint="$1"
  local label="$2"
  local http_code

  http_code="$(
    cpanel_curl \
      --silent \
      --show-error \
      --max-time 90 \
      --output "${response_file}" \
      --write-out '%{http_code}' \
      "${host}/${endpoint}"
  )"
  if [[ "${http_code}" != "200" ]] ||
    ! jq -e '.status == 1' "${response_file}" >/dev/null; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{status,errors,messages}' "${response_file}" 2>/dev/null || true)" >&2
    exit 1
  fi
}

cpanel_require_expected_version \
  "${host}" \
  "${CPANEL_EXPECTED_VERSION}" \
  "${response_file}"

request 'execute/Tokens/list' 'Active API token verification'
if ! jq -e \
  --arg name "${CPANEL_API_TOKEN_NAME}" \
  '[.data[] | select(.name == $name)] | length == 1' \
  "${response_file}" >/dev/null; then
  printf '%s\n' \
    'Refusing destructive cPanel operations: CPANEL_API_TOKEN_NAME is not present exactly once on the account.' >&2
  exit 1
fi

request \
  'execute/Fileman/list_files?dir=&show_hidden=1&limit=1000' \
  'Acceptance account marker inventory'
if ! jq -e \
  --arg marker_file "${marker_file}" \
  '[.data[] | select(.type == "file" and .file == $marker_file)] | length == 1' \
  "${response_file}" >/dev/null; then
  printf '%s\n' \
    'Refusing destructive cPanel operations: the dedicated-account marker file is missing.' >&2
  exit 1
fi

request \
  "execute/Fileman/get_file_content?dir=&file=${marker_file}&from_charset=UTF-8&to_charset=UTF-8&update_html_document_encoding=0" \
  'Acceptance account marker verification'
if ! jq -e \
  --arg expected_marker "${expected_marker}" \
  '.data.content == $expected_marker' \
  "${response_file}" >/dev/null; then
  printf '%s\n' \
    'Refusing destructive cPanel operations: the dedicated-account marker does not match the configured account and active token.' >&2
  exit 1
fi
