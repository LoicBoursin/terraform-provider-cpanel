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

if [[ "${CPANEL_INITIALIZE_TEST_ACCOUNT:-}" != "1" ]]; then
  printf '%s\n' \
    'Set CPANEL_INITIALIZE_TEST_ACCOUNT=1 to initialize the dedicated cPanel acceptance account.' >&2
  exit 1
fi
if [[ "${CPANEL_ACCEPT_DESTRUCTIVE:-}" != "1" ]]; then
  printf '%s\n' \
    'CPANEL_ACCEPT_DESTRUCTIVE=1 is required for dedicated-account initialization.' >&2
  exit 1
fi
if [[ "${CPANEL_HOST%/}" != "${CPANEL_EXPECTED_TEST_HOST%/}" ]] ||
  [[ "${CPANEL_USERNAME}" != "${CPANEL_EXPECTED_TEST_USERNAME}" ]]; then
  printf '%s\n' \
    'Configured cPanel host or username does not match the dedicated test account.' >&2
  exit 1
fi
if [[ "${CPANEL_API_TOKEN_NAME}" == tfcpaneltoken* ]]; then
  printf '%s\n' \
    'The active API token must not use the reserved acceptance-test prefix.' >&2
  exit 1
fi

for command in curl jq; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "${command}" >&2
    exit 1
  fi
done

host="${CPANEL_HOST%/}"
marker="$(
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

cpanel_require_expected_version \
  "${host}" \
  "${CPANEL_EXPECTED_VERSION}" \
  "${response_file}"

http_code="$(
  cpanel_curl \
    --silent \
    --show-error \
    --max-time 90 \
    --output "${response_file}" \
    --write-out '%{http_code}' \
    "${host}/execute/Tokens/list"
)"
if [[ "${http_code}" != "200" ]] ||
  ! jq -e \
    --arg name "${CPANEL_API_TOKEN_NAME}" \
    '.status == 1 and ([.data[] | select(.name == $name)] | length == 1)' \
    "${response_file}" >/dev/null; then
  printf '%s\n' \
    'Configured active API token name was not found exactly once.' >&2
  exit 1
fi

http_code="$(
  cpanel_curl \
    --silent \
    --show-error \
    --max-time 90 \
    --request POST \
    --output "${response_file}" \
    --write-out '%{http_code}' \
    --header 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode "content=${marker}" \
    --data-urlencode 'dir=' \
    --data-urlencode 'fallback=0' \
    --data-urlencode "file=${marker_file}" \
    --data-urlencode 'from_charset=UTF-8' \
    --data-urlencode 'to_charset=UTF-8' \
    "${host}/execute/Fileman/save_file_content"
)"
if [[ "${http_code}" != "200" ]] ||
  ! jq -e '.status == 1 and (.data.path | strings | length > 0)' \
    "${response_file}" >/dev/null; then
  printf 'Unable to initialize acceptance account marker: %s\n' \
    "$(jq -c '{status,errors,messages}' "${response_file}" 2>/dev/null || true)" >&2
  exit 1
fi

"${script_directory}/cpanel-verify-test-account.sh"
printf '%s\n' 'Dedicated cPanel acceptance account initialized'
