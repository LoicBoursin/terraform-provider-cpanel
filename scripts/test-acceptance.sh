#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
env_file="${CPANEL_ENV_FILE:-${HOME}/.config/terraform-provider-cpanel/acceptance.env}"
baseline_file="${CPANEL_BASELINE_FILE:-${env_file%.env}-baseline.env}"
artifact_manifest="${CPANEL_TEST_ARTIFACT_MANIFEST:-${env_file%.env}-artifacts.txt}"
export CPANEL_TEST_ARTIFACT_MANIFEST="${artifact_manifest}"
remote_artifact_manifest="${CPANEL_TEST_REMOTE_ARTIFACT_MANIFEST:-.terraform-provider-cpanel-acceptance-artifacts}"
export CPANEL_TEST_REMOTE_ARTIFACT_MANIFEST="${remote_artifact_manifest}"

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
if [[ -f "${baseline_file}" ]]; then
  cpanel_load_environment_file \
    "${baseline_file}" \
    'Acceptance baseline file' \
    CPANEL_EXPECTED_LOCALE \
    CPANEL_EXPECTED_LOG_ARCHIVE \
    CPANEL_EXPECTED_LOG_PRUNE \
    CPANEL_EXPECTED_LOG_RETENTION \
    CPANEL_EXPECTED_NOTIFICATION_PREFERENCES \
    CPANEL_EXPECTED_SPAM_PREFERENCES \
    CPANEL_EXPECTED_GPG_PUBLIC_COUNT \
    CPANEL_EXPECTED_GPG_SECRET_COUNT \
    CPANEL_EXPECTED_SSH_PUBLIC_COUNT \
    CPANEL_ALLOW_GPG_KEYPAIR_DELETE
fi

for variable in \
  CPANEL_HOST \
  CPANEL_USERNAME \
  CPANEL_API_TOKEN \
  CPANEL_API_TOKEN_NAME \
  CPANEL_EXPECTED_TEST_HOST \
  CPANEL_EXPECTED_TEST_USERNAME \
  CPANEL_EXPECTED_VERSION \
  CPANEL_EXPECTED_LOCALE \
  CPANEL_EXPECTED_LOG_ARCHIVE \
  CPANEL_EXPECTED_LOG_PRUNE \
  CPANEL_EXPECTED_LOG_RETENTION \
  CPANEL_EXPECTED_NOTIFICATION_PREFERENCES \
  CPANEL_EXPECTED_SPAM_PREFERENCES \
  CPANEL_EXPECTED_GPG_PUBLIC_COUNT \
  CPANEL_EXPECTED_GPG_SECRET_COUNT \
  CPANEL_EXPECTED_SSH_PUBLIC_COUNT; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'Missing required environment variable: %s\n' "${variable}" >&2
    exit 1
  fi
done

"${script_directory}/cpanel-verify-test-account.sh"

if [[ "${remote_artifact_manifest}" != ".terraform-provider-cpanel-acceptance-artifacts" ]]; then
  printf 'Invalid remote acceptance artifact manifest name: %s\n' \
    "${remote_artifact_manifest}" >&2
  exit 1
fi

if [[ "${CPANEL_ALLOW_GPG_KEYPAIR_DELETE:-}" != "1" ]]; then
  printf '%s\n' \
    'CPANEL_ALLOW_GPG_KEYPAIR_DELETE=1 is required for the complete acceptance suite on a dedicated test account.' >&2
  exit 1
fi

clear_remote_artifact_manifest() {
  local host="${CPANEL_HOST%/}"
  local http_code
  local response_file

  response_file="$(mktemp)"
  http_code="$(
    cpanel_curl \
      --silent \
      --show-error \
      --max-time 90 \
      --request POST \
      --output "${response_file}" \
      --write-out '%{http_code}' \
      --header 'Content-Type: application/x-www-form-urlencoded' \
      --data-urlencode 'content=' \
      --data-urlencode 'dir=' \
      --data-urlencode 'fallback=0' \
      --data-urlencode "file=${remote_artifact_manifest}" \
      --data-urlencode 'from_charset=UTF-8' \
      --data-urlencode 'to_charset=UTF-8' \
      "${host}/execute/Fileman/save_file_content"
  )"
  if [[ "${http_code}" != "200" ]] ||
    ! jq -e '.status == 1 and (.data.path | strings | length > 0)' \
      "${response_file}" >/dev/null; then
    printf 'Unable to clear remote acceptance artifact manifest: %s\n' \
      "$(jq -c '{status,errors,messages,data}' "${response_file}" 2>/dev/null || true)" >&2
    rm -f "${response_file}"
    return 1
  fi

  http_code="$(
    cpanel_curl \
      --silent \
      --show-error \
      --max-time 90 \
      --output "${response_file}" \
      --write-out '%{http_code}' \
      "${host}/execute/Fileman/get_file_content?dir=&file=${remote_artifact_manifest}&from_charset=UTF-8&to_charset=UTF-8&update_html_document_encoding=0"
  )"
  if [[ "${http_code}" != "200" ]] ||
    ! jq -e '.status == 1 and .data.content == ""' \
      "${response_file}" >/dev/null; then
    printf 'Unable to verify empty remote acceptance artifact manifest: %s\n' \
      "$(jq -c '{status,errors,messages,data}' "${response_file}" 2>/dev/null || true)" >&2
    rm -f "${response_file}"
    return 1
  fi

  rm -f "${response_file}"
}

finalizer_active=1
# shellcheck disable=SC2329
finalize() {
  local command_status=$?
  local clear_status=0
  local cleanup_status
  local inventory_status
  local restore_status

  trap - EXIT
  if [[ "${finalizer_active}" != "1" ]]; then
    exit "${command_status}"
  fi

  set +e
  "${script_directory}/cpanel-restore-test-singletons.sh"
  restore_status=$?
  "${script_directory}/cpanel-clean-test-artifacts.sh"
  cleanup_status=$?
  CPANEL_REQUIRE_EMPTY=1 "${script_directory}/cpanel-smoke.sh"
  inventory_status=$?
  if [[ "${inventory_status}" == "0" ]]; then
    clear_remote_artifact_manifest
    clear_status=$?
  fi
  set -e

  if [[ "${command_status}" != "0" ]]; then
    exit "${command_status}"
  fi
  if [[ "${restore_status}" != "0" ]]; then
    exit "${restore_status}"
  fi
  if [[ "${cleanup_status}" != "0" ]]; then
    exit "${cleanup_status}"
  fi
  if [[ "${clear_status}" != "0" ]]; then
    exit "${clear_status}"
  fi
  if [[ "${inventory_status}" == "0" ]]; then
    rm -f "${artifact_manifest}"
  fi
  exit "${inventory_status}"
}

trap finalize EXIT

"${script_directory}/cpanel-restore-test-singletons.sh"
"${script_directory}/cpanel-clean-test-artifacts.sh"
CPANEL_REQUIRE_EMPTY=1 "${script_directory}/cpanel-smoke.sh"
umask 077
mkdir -p "$(dirname "${artifact_manifest}")"
: >"${artifact_manifest}"
chmod 600 "${artifact_manifest}"
clear_remote_artifact_manifest

cd "${repository_directory}"

export CGO_ENABLED="${CGO_ENABLED:-0}"
export TF_ACC=1

set +e
go test ./internal/provider -v -count=1 -timeout 105m "$@"
test_status=$?
set -e

exit "${test_status}"
