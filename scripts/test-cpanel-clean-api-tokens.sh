#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
temporary_directory="$(mktemp -d)"
artifact_manifest="${temporary_directory}/artifacts.jsonl"
response_file="${temporary_directory}/response.json"
revocation_log="${temporary_directory}/revocations.txt"
mock_inventory='{"status":1,"data":[{"name":"tfcpaneltokencandidate","create_time":"11","expires_at":"","has_full_access":"1","features":[],"whitelist_ips":[]}]}'

cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

source "${script_directory}/cpanel-common.sh"

printf '%s\n' \
  '{"version":1,"kind":"api_token_candidate","name":"tfcpaneltokencandidate","not_before":10,"not_after":20}' \
  >"${artifact_manifest}"

get_request() {
  if [[ "$1" != 'execute/Tokens/list' ]]; then
    printf 'Unexpected mocked GET request: %s\n' "$1" >&2
    return 1
  fi
  printf '%s\n' "${mock_inventory}" >"${response_file}"
}

uapi_post() {
  if [[ "$1" != 'Tokens' || "$2" != 'revoke' ||
    "$4" != 'name=tfcpaneltokencandidate' ]]; then
    printf 'Unexpected mocked UAPI request: %s\n' "$*" >&2
    return 1
  fi
  printf '%s\n' "$4" >>"${revocation_log}"
}

deleted="$(cpanel_cleanup_test_api_tokens \
  "${artifact_manifest}" \
  'active-provider-token' \
  "${response_file}")"
if [[ "${deleted}" != '1' ]] ||
  [[ "$(cat "${revocation_log}")" != 'name=tfcpaneltokencandidate' ]]; then
  printf 'Candidate manifest did not authorize exactly one API-token revocation\n' >&2
  exit 1
fi

: >"${revocation_log}"
if cpanel_cleanup_test_api_tokens \
  "${artifact_manifest}" \
  'tfcpaneltokencandidate' \
  "${response_file}" >/dev/null 2>&1; then
  printf 'API-token cleanup accepted the active provider token\n' >&2
  exit 1
fi
if [[ -s "${revocation_log}" ]]; then
  printf 'API-token cleanup attempted to revoke the active provider token\n' >&2
  exit 1
fi

mock_inventory='{"status":1,"data":{}}'
if cpanel_cleanup_test_api_tokens \
  "${artifact_manifest}" \
  'active-provider-token' \
  "${response_file}" >/dev/null 2>&1; then
  printf 'API-token cleanup accepted a malformed inventory\n' >&2
  exit 1
fi
if [[ -s "${revocation_log}" ]]; then
  printf 'API-token cleanup revoked a token from a malformed inventory\n' >&2
  exit 1
fi

mock_inventory='{"status":1,"data":[{"name":"tfcpaneltokencandidate","create_time":"21","expires_at":"","has_full_access":"1","features":[],"whitelist_ips":[]}]}'
if cpanel_cleanup_test_api_tokens \
  "${artifact_manifest}" \
  'active-provider-token' \
  "${response_file}" >/dev/null 2>&1; then
  printf 'API-token cleanup accepted a candidate created after its window\n' >&2
  exit 1
fi
if [[ -s "${revocation_log}" ]]; then
  printf 'API-token cleanup revoked a candidate created after its window\n' >&2
  exit 1
fi
