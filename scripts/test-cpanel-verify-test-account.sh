#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=cpanel-common.sh
source "${script_directory}/cpanel-common.sh"

temporary_directory="$(mktemp -d)"
cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

cat >"${temporary_directory}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

output_file=""
url=""
while (($# > 0)); do
  case "$1" in
    --config)
      shift 2
      ;;
    --output | --write-out | --max-time)
      if [[ "$1" == "--output" ]]; then
        output_file="$2"
      fi
      shift 2
      ;;
    --silent | --show-error)
      shift
      ;;
    *)
      url="$1"
      shift
      ;;
  esac
done

case "${url}" in
  */execute/StatsBar/get_stats?display=cpanelversion)
    printf '{"status":1,"data":[{"name":"cpanelversion","value":"%s"}]}\n' \
      "${CPANEL_VERIFY_TEST_VERSION}" >"${output_file}"
    ;;
  */execute/Tokens/list)
    printf '{"status":1,"data":[{"name":"%s"}]}\n' \
      "${CPANEL_VERIFY_TEST_TOKEN_NAME}" >"${output_file}"
    ;;
  */execute/Fileman/list_files?*)
    printf '{"status":1,"data":[{"type":"file","file":".terraform-provider-cpanel-acceptance-account"}]}\n' \
      >"${output_file}"
    ;;
  */execute/Fileman/get_file_content?*)
    jq -n \
      --arg content "${CPANEL_VERIFY_TEST_MARKER}" \
      '{status: 1, data: {content: $content}}' >"${output_file}"
    ;;
  *)
    printf 'Unexpected test URL: %s\n' "${url}" >&2
    exit 1
    ;;
esac

printf '200'
EOF
chmod 0700 "${temporary_directory}/curl"

host="https://cpanel.example.test:2083"
username="test-account"
api_token="test-token"
api_token_name="active-token"
expected_marker="$(
  cpanel_acceptance_account_marker \
    "${host}" \
    "${username}" \
    "${api_token}"
)"

run_verification() {
  CPANEL_ENV_FILE="${temporary_directory}/missing.env" \
    CPANEL_HOST="${host}" \
    CPANEL_USERNAME="${username}" \
    CPANEL_API_TOKEN="${api_token}" \
    CPANEL_API_TOKEN_NAME="${api_token_name}" \
    CPANEL_EXPECTED_TEST_HOST="${host}" \
    CPANEL_EXPECTED_TEST_USERNAME="${username}" \
    CPANEL_EXPECTED_VERSION="134.0 (build 45)" \
    CPANEL_ACCEPT_DESTRUCTIVE=1 \
    CPANEL_VERIFY_TEST_VERSION="${3:-134.0 (build 45)}" \
    CPANEL_VERIFY_TEST_TOKEN_NAME="${1}" \
    CPANEL_VERIFY_TEST_MARKER="${2}" \
    PATH="${temporary_directory}:${PATH}" \
    "${script_directory}/cpanel-verify-test-account.sh"
}

run_verification "${api_token_name}" "${expected_marker}"

if run_verification "another-token" "${expected_marker}" >/dev/null 2>&1; then
  printf 'Expected a missing active token name to be rejected\n' >&2
  exit 1
fi

if run_verification "${api_token_name}" "wrong-marker" >/dev/null 2>&1; then
  printf 'Expected a mismatched account marker to be rejected\n' >&2
  exit 1
fi

if run_verification \
  "${api_token_name}" \
  "${expected_marker}" \
  "135.0 (build 1)" >/dev/null 2>&1; then
  printf 'Expected a mismatched cPanel version to be rejected\n' >&2
  exit 1
fi
