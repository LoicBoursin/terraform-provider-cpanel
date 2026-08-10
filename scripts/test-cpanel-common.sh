#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=cpanel-common.sh
source "${script_directory}/cpanel-common.sh"

for host in \
  'https://cpanel.example.test:2083' \
  'https://cpanel.example.test:2083/' \
  'https://cpanel.example.test:2083/base' \
  'https://192.0.2.10:2083' \
  'https://[2001:db8::1]:2083'; do
  cpanel_validate_host "${host}"
done

for host in \
  '' \
  'http://cpanel.example.test:2083' \
  'https://' \
  'https://:2083' \
  'https://cpanel.example.test:' \
  'https://cpanel.example.test:notaport' \
  'https://cpanel.example.test:0' \
  'https://cpanel.example.test:65536' \
  'https://-cpanel.example.test:2083' \
  'https://cpanel..example.test:2083' \
  'https://2001:db8::1:2083' \
  'https://[example.test]:2083' \
  'https://[::::]:2083' \
  'https://[2001:db8::1::2]:2083' \
  'https://user:secret@cpanel.example.test:2083' \
  'https://cpanel.example.test:2083/?token=secret' \
  'https://cpanel.example.test:2083/#fragment' \
  'https://cpanel.example.test:2083/ bad'; do
  if cpanel_validate_host "${host}" >/dev/null 2>&1; then
    printf 'Expected invalid cPanel host to be rejected: %s\n' "${host}" >&2
    exit 1
  fi
done

temporary_directory="$(mktemp -d)"
cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

private_file="${temporary_directory}/private.env"
printf 'TOKEN=value\n' >"${private_file}"
chmod 0600 "${private_file}"
cpanel_require_private_file "${private_file}" 'Private test file'

chmod 0640 "${private_file}"
if cpanel_require_private_file "${private_file}" 'Private test file' >/dev/null 2>&1; then
  printf 'Expected group-readable private file to be rejected\n' >&2
  exit 1
fi

chmod 0600 "${private_file}"
ln -s "${private_file}" "${temporary_directory}/private-link.env"
if cpanel_require_private_file \
  "${temporary_directory}/private-link.env" \
  'Private test file' >/dev/null 2>&1; then
  printf 'Expected private file symlink to be rejected\n' >&2
  exit 1
fi

environment_file="${temporary_directory}/acceptance.env"
printf '%s\n' \
  'CPANEL_HOST=https://cpanel.example.test:2083' \
  'CPANEL_USERNAME="test-user"' \
  "CPANEL_API_TOKEN='test-token'" \
  "CPANEL_EXPECTED_VERSION='134.0 (build 45)'" >"${environment_file}"
chmod 0600 "${environment_file}"
unset CPANEL_HOST CPANEL_USERNAME CPANEL_API_TOKEN CPANEL_EXPECTED_VERSION
cpanel_load_environment_file \
  "${environment_file}" \
  'Acceptance environment file' \
  CPANEL_HOST \
  CPANEL_USERNAME \
  CPANEL_API_TOKEN \
  CPANEL_EXPECTED_VERSION
if [[
  "${CPANEL_HOST}" != "https://cpanel.example.test:2083"
  || "${CPANEL_USERNAME}" != "test-user"
  || "${CPANEL_API_TOKEN}" != "test-token"
  || "${CPANEL_EXPECTED_VERSION}" != "134.0 (build 45)"
]]; then
  printf 'Acceptance environment file was not parsed correctly\n' >&2
  exit 1
fi

printf '%s\n' \
  'CPANEL_HOST=https://cpanel.example.test:2083' \
  'CPANEL_EXPECTED_LOCALE=fr' >"${environment_file}"
if cpanel_load_environment_file \
  "${environment_file}" \
  'Acceptance baseline file' \
  CPANEL_EXPECTED_LOCALE >/dev/null 2>&1; then
  printf 'Expected unsupported acceptance variable to be rejected\n' >&2
  exit 1
fi

# shellcheck disable=SC2016
printf '%s\n' 'CPANEL_API_TOKEN=$(printf unsafe)' >"${environment_file}"
if cpanel_load_environment_file \
  "${environment_file}" \
  'Acceptance environment file' \
  CPANEL_API_TOKEN >/dev/null 2>&1; then
  printf 'Expected shell expression in acceptance value to be rejected\n' >&2
  exit 1
fi

expected_digest="53e1ae6988b7b957644f1eacbf83f0e437678beca90239b1d5a90a12d79d2bc8"
if [[ "$(cpanel_sha256_text 'terraform-provider-cpanel')" != "${expected_digest}" ]]; then
  printf 'cpanel_sha256_text returned an unexpected digest\n' >&2
  exit 1
fi

manifest_file="${temporary_directory}/acceptance-artifacts.txt"
cat >"${manifest_file}" <<'EOF'
{"version":1,"kind":"value","value":"tfcpanelfixture"}
{"version":1,"kind":"dns_record","zone":"example.test","name":"fixture","type":"TXT","ttl":300,"data":["value"]}
{"version":1,"kind":"api_token","name":"tfcpaneltokenfixture","created_at":1,"expires_at":0,"has_full_access":0,"features":[],"whitelist_ips":[]}
{"version":1,"kind":"api_token_candidate","name":"tfcpaneltokencandidate","not_before":10,"not_after":20}
EOF
chmod 0600 "${manifest_file}"
cpanel_validate_artifact_manifest "${manifest_file}" 'Test artifact manifest'
if ! cpanel_artifact_registered "${manifest_file}" "tfcpanelfixture"; then
  printf 'Expected generic artifact to be registered\n' >&2
  exit 1
fi
token_identity="$(sed -n '3p' "${manifest_file}")"
if ! cpanel_api_token_artifact_registered \
  "${manifest_file}" \
  "${token_identity}"; then
  printf 'Expected API token identity to be registered\n' >&2
  exit 1
fi
candidate_identity='{"version":1,"kind":"api_token","name":"tfcpaneltokencandidate","created_at":11,"expires_at":0,"has_full_access":1,"features":[],"whitelist_ips":[]}'
if ! cpanel_api_token_candidate_registered \
  "${manifest_file}" \
  "${candidate_identity}"; then
  printf 'Expected API token candidate to authorize the matching token\n' >&2
  exit 1
fi
for invalid_candidate in \
  '{"version":1,"kind":"api_token","name":"tfcpaneltokencandidate","created_at":9,"expires_at":0,"has_full_access":1,"features":[],"whitelist_ips":[]}' \
  '{"version":1,"kind":"api_token","name":"tfcpaneltokencandidate","created_at":21,"expires_at":0,"has_full_access":1,"features":[],"whitelist_ips":[]}' \
  '{"version":1,"kind":"api_token","name":"tfcpaneltokencandidate","created_at":11,"expires_at":0,"has_full_access":0,"features":[],"whitelist_ips":[]}' \
  '{"version":1,"kind":"api_token","name":"tfcpaneltokencandidate","created_at":11,"expires_at":0,"has_full_access":1,"features":["feature"],"whitelist_ips":[]}' \
  '{"version":1,"kind":"api_token","name":"tfcpaneltokenother","created_at":11,"expires_at":0,"has_full_access":1,"features":[],"whitelist_ips":[]}'; do
  if cpanel_api_token_candidate_registered \
    "${manifest_file}" \
    "${invalid_candidate}"; then
    printf 'Expected unsafe API token candidate identity to be rejected\n' >&2
    exit 1
  fi
done
dns_record_identity="$(
  sed -n '2p' "${manifest_file}"
)"
if ! cpanel_dns_record_artifact_registered \
  "${manifest_file}" \
  "${dns_record_identity}"; then
  printf 'Expected DNS record identity to be registered\n' >&2
  exit 1
fi
if cpanel_dns_record_artifact_registered \
  "${manifest_file}" \
  '{"version":1,"kind":"dns_record","zone":"example.test","name":"fixture","type":"TXT","ttl":600,"data":["value"]}'; then
  printf 'Expected changed DNS record identity to be rejected\n' >&2
  exit 1
fi

cat >"${manifest_file}" <<'EOF'
{"version":1,"kind":"value","value":"invalid","unexpected":true}
{"version":1,"kind":"value","value":"valid-last-record"}
EOF
if cpanel_validate_artifact_manifest \
  "${manifest_file}" \
  'Invalid artifact manifest' >/dev/null 2>&1; then
  printf 'Expected an invalid earlier manifest record to be rejected\n' >&2
  exit 1
fi

printf '{"version":1,"kind":"value","value":"%04100d"}\n' 0 >"${manifest_file}"
if cpanel_validate_artifact_manifest \
  "${manifest_file}" \
  'Oversized artifact manifest' >/dev/null 2>&1; then
  printf 'Expected an oversized manifest record to be rejected\n' >&2
  exit 1
fi

ftp_inventory_file="${temporary_directory}/ftp-inventory.json"
ftp_quota_file="${temporary_directory}/ftp-quota.json"

printf '{"status":1,"data":[]}\n' >"${ftp_inventory_file}"
cpanel_ftp_home_inventory_is_safe_to_delete "${ftp_inventory_file}"

printf '%s\n' \
  '{"status":1,"data":[{"file":".ftpquota","type":"file","size":"4"}]}' \
  >"${ftp_inventory_file}"
cpanel_ftp_home_inventory_is_safe_to_delete "${ftp_inventory_file}"

printf '%s\n' \
  '{"status":1,"data":[{"file":"content.txt","type":"file","size":"4"}]}' \
  >"${ftp_inventory_file}"
if cpanel_ftp_home_inventory_is_safe_to_delete \
  "${ftp_inventory_file}" >/dev/null 2>&1; then
  printf 'Expected an FTP home directory with user content to be rejected\n' >&2
  exit 1
fi

printf '{"status":1,"data":{"content":"0 0\\n"}}\n' >"${ftp_quota_file}"
cpanel_ftp_quota_file_is_empty "${ftp_quota_file}"

printf '{"status":1,"data":{"content":"1 0\\n"}}\n' >"${ftp_quota_file}"
if cpanel_ftp_quota_file_is_empty "${ftp_quota_file}" >/dev/null 2>&1; then
  printf 'Expected a non-empty FTP quota file to be rejected\n' >&2
  exit 1
fi

ssl_certificate_file="${temporary_directory}/ssl-certificates.json"
cat >"${ssl_certificate_file}" <<'EOF'
{"status":1,"data":[{"id":"1","friendly_name":"tfcpanelsslcertfixture","domains":["example.test"]},{"id":"automatic-id","friendly_name":"Cert for “tfcpaneladdonfixture.example.test”","domains":["tfcpaneladdonfixture.example.test"]}]}
EOF
if [[ "$(
  cpanel_ssl_certificate_cleanup_artifact \
    "${ssl_certificate_file}" \
    "1"
)" != "tfcpanelsslcertfixture" ]]; then
  printf 'Expected a provider-managed SSL certificate to use its friendly name\n' >&2
  exit 1
fi
if [[ "$(
  cpanel_ssl_certificate_cleanup_artifact \
    "${ssl_certificate_file}" \
    "automatic-id"
)" != "tfcpaneladdonfixture.example.test" ]]; then
  printf 'Expected an automatic SSL certificate to use its registered domain\n' >&2
  exit 1
fi
if cpanel_ssl_certificate_cleanup_artifact \
  "${ssl_certificate_file}" \
  "missing" >/dev/null 2>&1; then
  printf 'Expected a missing SSL certificate identity to be rejected\n' >&2
  exit 1
fi

cat >"${temporary_directory}/curl" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" >"${CPANEL_CURL_TEST_ARGUMENTS}"
cat >"${CPANEL_CURL_TEST_CONFIG}"
EOF
chmod 0700 "${temporary_directory}/curl"

export CPANEL_USERNAME='test-user'
export CPANEL_API_TOKEN='test-token-that-must-not-appear-in-argv'
export CPANEL_CURL_TEST_ARGUMENTS="${temporary_directory}/arguments"
export CPANEL_CURL_TEST_CONFIG="${temporary_directory}/config"
PATH="${temporary_directory}:${PATH}" cpanel_curl \
  --silent \
  'https://cpanel.example.test:2083/execute/Version/get_version'

if [[ "$(<"${CPANEL_CURL_TEST_ARGUMENTS}")" == *"${CPANEL_API_TOKEN}"* ]]; then
  printf 'cPanel API token was exposed in curl arguments\n' >&2
  exit 1
fi
if [[ "$(<"${CPANEL_CURL_TEST_CONFIG}")" != *"Authorization: cpanel ${CPANEL_USERNAME}:${CPANEL_API_TOKEN}"* ]]; then
  printf 'cPanel authorization header was not passed through curl config\n' >&2
  exit 1
fi
