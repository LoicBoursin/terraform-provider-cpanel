#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
temporary_directory="$(mktemp -d)"
fixture_directory="${temporary_directory}/fixture"

cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

mkdir -p "${fixture_directory}/scripts"
cp "${script_directory}/cpanel-ci-acceptance.sh" \
  "${fixture_directory}/scripts/cpanel-ci-acceptance.sh"
cp "${script_directory}/cpanel-ci-baseline.env" \
  "${fixture_directory}/scripts/cpanel-ci-baseline.env"

cat >"${fixture_directory}/scripts/tool-versions.sh" <<'EOF'
cpanel_test_version='999.1 (build 2)'
EOF
cat >"${fixture_directory}/scripts/test-acceptance.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

[[ "${CPANEL_EXPECTED_TEST_HOST}" == "https://cpanel.example.test:2083" ]]
[[ "${CPANEL_EXPECTED_TEST_USERNAME}" == "test-account" ]]
[[ "${CPANEL_EXPECTED_VERSION}" == "999.1 (build 2)" ]]
[[ "${CPANEL_ACCEPT_DESTRUCTIVE}" == "1" ]]
[[ "$(stat -f '%Lp' "${CPANEL_BASELINE_FILE}" 2>/dev/null || stat -c '%a' "${CPANEL_BASELINE_FILE}")" == "600" ]]
grep -q '^CPANEL_EXPECTED_LOCALE=' "${CPANEL_BASELINE_FILE}"
grep -q '^CPANEL_ALLOW_GPG_KEYPAIR_DELETE=1$' "${CPANEL_BASELINE_FILE}"
[[ "${1:-}" == "fixture-argument" ]]
printf '%s\n' "${CPANEL_BASELINE_FILE}"
EOF
chmod 0755 \
  "${fixture_directory}/scripts/cpanel-ci-acceptance.sh" \
  "${fixture_directory}/scripts/test-acceptance.sh"

for missing_variable in \
  CPANEL_HOST \
  CPANEL_USERNAME \
  CPANEL_API_TOKEN \
  CPANEL_API_TOKEN_NAME; do
  environment=(
    "CPANEL_HOST=https://cpanel.example.test:2083"
    "CPANEL_USERNAME=test-account"
    "CPANEL_API_TOKEN=test-token"
    "CPANEL_API_TOKEN_NAME=active-token"
  )
  filtered_environment=()
  for assignment in "${environment[@]}"; do
    if [[ "${assignment%%=*}" != "${missing_variable}" ]]; then
      filtered_environment+=("${assignment}")
    fi
  done
  if env -i \
    "PATH=${PATH}" \
    "HOME=${temporary_directory}" \
    "${filtered_environment[@]}" \
    "${fixture_directory}/scripts/cpanel-ci-acceptance.sh" \
    fixture-argument >/dev/null 2>&1; then
    printf 'CI acceptance accepted missing secret: %s\n' \
      "${missing_variable}" >&2
    exit 1
  fi
done

baseline_file="$(
  env -i \
    "PATH=${PATH}" \
    "HOME=${temporary_directory}" \
    'CPANEL_HOST=https://cpanel.example.test:2083' \
    'CPANEL_USERNAME=test-account' \
    'CPANEL_API_TOKEN=test-token' \
    'CPANEL_API_TOKEN_NAME=active-token' \
    "${fixture_directory}/scripts/cpanel-ci-acceptance.sh" \
    fixture-argument
)"
if [[ -e "${baseline_file}" ]]; then
  printf 'Temporary CI acceptance baseline was not removed: %s\n' \
    "${baseline_file}" >&2
  exit 1
fi

printf '%s\n' 'cPanel CI acceptance environment tests passed'
