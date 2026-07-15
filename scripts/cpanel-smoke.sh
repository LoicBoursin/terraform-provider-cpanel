#!/usr/bin/env bash

set -euo pipefail

env_file="${CPANEL_ENV_FILE:-${HOME}/.config/terraform-provider-cpanel/acceptance.env}"

if [[ -f "${env_file}" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${env_file}"
  set +a
fi

for variable in CPANEL_HOST CPANEL_USERNAME CPANEL_API_TOKEN; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'Missing required environment variable: %s\n' "${variable}" >&2
    exit 1
  fi
done

for command in curl jq; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    printf 'Missing required command: %s\n' "${command}" >&2
    exit 1
  fi
done

host="${CPANEL_HOST%/}"
authorization="Authorization: cpanel ${CPANEL_USERNAME}:${CPANEL_API_TOKEN}"
response_file="$(mktemp)"

cleanup() {
  rm -f "${response_file}"
}

trap cleanup EXIT

request() {
  local endpoint="$1"
  local label="$2"
  local status
  local http_code

  http_code="$(
    curl \
      --silent \
      --show-error \
      --max-time 90 \
      --output "${response_file}" \
      --write-out '%{http_code}' \
      --header "${authorization}" \
      "${host}/${endpoint}"
  )"

  if [[ "${http_code}" != "200" ]]; then
    printf '%s failed with HTTP %s\n' "${label}" "${http_code}" >&2
    exit 1
  fi

  status="$(jq -r '.status // .cpanelresult.event.result // empty' "${response_file}")"
  if [[ "${status}" != "1" ]]; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{errors, messages, cpanelresult}' "${response_file}")" >&2
    exit 1
  fi
}

request \
  'execute/StatsBar/get_stats?display=cpanelversion' \
  'cPanel version check'
cpanel_version="$(
  jq -r '.data[] | select(.name == "cpanelversion") | .value' "${response_file}"
)"

request 'execute/Locale/get_attributes' 'cPanel locale check'
account_locale="$(jq -r '.data.locale // empty' "${response_file}")"
account_locale_direction="$(jq -r '.data.direction // empty' "${response_file}")"
account_locale_encoding="$(jq -r '.data.encoding // empty' "${response_file}")"
if [[
  -z "${account_locale}"
  || -z "${account_locale_direction}"
  || -z "${account_locale_encoding}"
 ]]; then
  printf 'cPanel returned incomplete locale attributes\n' >&2
  exit 1
fi

request 'execute/Locale/list_locales' 'cPanel locale inventory'
locale_count="$(jq -r '.data | length' "${response_file}")"
if ! jq -e --arg locale "${account_locale}" \
  '.data[] | select(.locale == $locale)' \
  "${response_file}" >/dev/null; then
  printf 'Current cPanel locale is absent from the locale inventory: %s\n' \
    "${account_locale}" >&2
  exit 1
fi
if [[
  -n "${CPANEL_EXPECTED_LOCALE:-}"
  && "${account_locale}" != "${CPANEL_EXPECTED_LOCALE}"
 ]]; then
  printf 'cPanel locale is %s; expected restored locale %s\n' \
    "${account_locale}" \
    "${CPANEL_EXPECTED_LOCALE}" >&2
  exit 1
fi

request 'execute/LogManager/get_settings' 'cPanel log settings check'
if ! jq -e \
  '
    ((.data.archive_logs | tostring) | test("^[01]$"))
    and ((.data.prune_archive | tostring) | test("^[01]$"))
    and ((.data.using_default | tostring) | test("^[01]$"))
    and ((.data.retention_days | tostring) | test("^[0-9]+$"))
  ' \
  "${response_file}" >/dev/null; then
  printf 'cPanel returned incomplete log archival settings\n' >&2
  exit 1
fi
log_archive_logs="$(jq -r '.data.archive_logs | tostring' "${response_file}")"
log_prune_archive="$(jq -r '.data.prune_archive | tostring' "${response_file}")"
log_using_default="$(jq -r '.data.using_default | tostring' "${response_file}")"
log_effective_retention="$(
  jq -r '.data.retention_days | tostring' "${response_file}"
)"
log_configured_retention="${log_effective_retention}"
if [[ "${log_using_default}" == "1" ]]; then
  log_configured_retention="-1"
fi
if [[
  -n "${CPANEL_EXPECTED_LOG_ARCHIVE:-}"
  && "${log_archive_logs}" != "${CPANEL_EXPECTED_LOG_ARCHIVE}"
 ]]; then
  printf 'cPanel archive_logs is %s; expected restored value %s\n' \
    "${log_archive_logs}" \
    "${CPANEL_EXPECTED_LOG_ARCHIVE}" >&2
  exit 1
fi
if [[
  -n "${CPANEL_EXPECTED_LOG_PRUNE:-}"
  && "${log_prune_archive}" != "${CPANEL_EXPECTED_LOG_PRUNE}"
 ]]; then
  printf 'cPanel prune_archive is %s; expected restored value %s\n' \
    "${log_prune_archive}" \
    "${CPANEL_EXPECTED_LOG_PRUNE}" >&2
  exit 1
fi
if [[
  -n "${CPANEL_EXPECTED_LOG_RETENTION:-}"
  && "${log_configured_retention}" != "${CPANEL_EXPECTED_LOG_RETENTION}"
 ]]; then
  printf 'cPanel log retention is %s; expected restored value %s\n' \
    "${log_configured_retention}" \
    "${CPANEL_EXPECTED_LOG_RETENTION}" >&2
  exit 1
fi

request 'execute/Features/list_features' 'feature check'
for feature in addondomains apitokens blockers cron dynamicdns ftpaccts handlers indexmanager mime modsecurity mysql parkeddomains passengerapps popaccts postgres redirects sslmanager subdomains version_control webprotect zoneedit; do
  if [[ "$(jq -r --arg feature "${feature}" '.data[$feature] // 0' "${response_file}")" != "1" ]]; then
    printf 'Required cPanel feature is disabled: %s\n' "${feature}" >&2
    exit 1
  fi
done

request 'execute/Variables/get_user_information' 'cPanel account home check'
account_home="$(jq -r '.data.home' "${response_file}")"
if [[ -z "${account_home}" || "${account_home}" != /* ]]; then
  printf 'cPanel returned an invalid account home directory\n' >&2
  exit 1
fi

request \
  "execute/DirectoryIndexes/get_indexing?dir=${account_home}/public_html" \
  'Directory Indexes check'
public_html_index_type="$(jq -r '.data' "${response_file}")"

request \
  "execute/DirectoryPrivacy/is_directory_protected?dir=${account_home}/public_html" \
  'Directory Privacy check'
public_html_protected="$(jq -r '.data.protected' "${response_file}")"

request 'execute/Tokens/list' 'API token check'
api_token_count="$(jq -r '.data | length' "${response_file}")"
api_test_token_count="$(
  jq -r \
    '[.data[].name | select(startswith("tfcpaneltoken"))] | length' \
    "${response_file}"
)"

request 'execute/DynamicDNS/list' 'Dynamic DNS check'
dynamic_dns_count="$(jq -r '.data | length' "${response_file}")"
dynamic_dns_test_count="$(
  jq -r \
    '[.data[].domain | select(startswith("tfcpanelddns"))] | length' \
    "${response_file}"
)"

request 'execute/Mime/list_redirects' 'HTTP redirect check'
redirect_count="$(jq -r '.data | length' "${response_file}")"
redirect_test_count="$(
  jq -r \
    '[.data[].source | select(startswith("/tfcpanelredirect-"))] | length' \
    "${response_file}"
)"

request 'execute/Mime/list_mime?type=user' 'custom MIME type check'
mime_type_count="$(jq -r '.data | length' "${response_file}")"
mime_type_test_count="$(
  jq -r \
    '[.data[].type | select(startswith("application/x-tfcpanel-"))] | length' \
    "${response_file}"
)"

request 'execute/Mime/list_handlers?type=user' 'Apache handler check'
apache_handler_count="$(jq -r '.data | length' "${response_file}")"
apache_handler_test_count="$(
  jq -r \
    '[.data[].extension | select(startswith(".tfcpanelhandler"))] | length' \
    "${response_file}"
)"

request 'execute/VersionControl/retrieve' 'Git repository check'
git_repository_count="$(jq -r '.data | length' "${response_file}")"
git_repository_test_count="$(
  jq -r \
    '[.data[]
      | .repository_root
      | split("/")
      | last
      | select(startswith("tfcpanel-git-"))] | length' \
    "${response_file}"
)"

request 'execute/PassengerApps/list_applications' 'Passenger application check'
passenger_application_count="$(jq -r '.data | length' "${response_file}")"
passenger_application_test_count="$(
  jq -r \
    '[.data
      | to_entries[]
      | .key
      | select(startswith("tfcpanelpassenger"))] | length' \
    "${response_file}"
)"

request 'execute/SSL/list_certs' 'stored SSL certificate check'
if ! jq -e \
  '
    (.data | type == "array")
    and all(
      .data[];
      (
        ((.id? | type) == "string" or (.id? | type) == "number")
        and ((.id | tostring | length) > 0)
        and (
          .domain_is_configured? == 0
          or .domain_is_configured? == "0"
          or .domain_is_configured? == false
          or .domain_is_configured? == "false"
          or .domain_is_configured? == 1
          or .domain_is_configured? == "1"
          or .domain_is_configured? == true
          or .domain_is_configured? == "true"
        )
      )
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Stored SSL certificate inventory is incomplete\n' >&2
  exit 1
fi
ssl_certificate_count="$(jq -r '.data | length' "${response_file}")"
ssl_certificate_test_count="$(
  jq -r \
    '[.data[]
      | select(
          (.friendly_name? | type) == "string"
          and (.friendly_name | startswith("tfcpanelsslcert"))
        )] | length' \
    "${response_file}"
)"

request 'execute/Postgresql/list_databases' 'PostgreSQL database check'
database_count="$(jq -r '.data | length' "${response_file}")"

request 'execute/Postgresql/list_users' 'PostgreSQL user check'
user_count="$(jq -r '.data | length' "${response_file}")"

test_prefix="${CPANEL_USERNAME}_tf"

request 'execute/Mysql/list_databases' 'MySQL database check'
mysql_database_count="$(jq -r '.data | length' "${response_file}")"
mysql_test_database_count="$(
  jq -r --arg prefix "${test_prefix}" \
    '[.data[].database | select(startswith($prefix))] | length' \
    "${response_file}"
)"

request 'execute/Mysql/list_users' 'MySQL user check'
mysql_user_count="$(jq -r '.data | length' "${response_file}")"
mysql_test_user_count="$(
  jq -r --arg prefix "${test_prefix}" \
    '[.data[].user | select(startswith($prefix))] | length' \
    "${response_file}"
)"

request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=MysqlFE&cpanel_jsonapi_func=listhosts" \
  'remote MySQL host check'
mysql_remote_host_count="$(jq -r '.cpanelresult.data | length' "${response_file}")"
mysql_test_remote_host_count="$(
  jq -r \
    '[.cpanelresult.data[].host
      | select(
          . == "198.51.100.245"
          or . == "198.51.100.246"
          or . == "198.51.100.247"
          or . == "198.51.100.248"
          or . == "198.51.100.249"
          or . == "198.51.100.250"
        )] | length' \
    "${response_file}"
)"

request 'execute/CPDAVD/list_delegates' 'Calendar delegate check'
if ! jq -e \
  '
    (.data | type == "array")
    and all(
      .data[];
      (.delegator? | type) == "string"
      and (.delegatee? | type) == "string"
      and (.calendar? | type) == "string"
      and (.calname? | type) == "string"
      and (
        .readonly? == 0
        or .readonly? == "0"
        or .readonly? == false
        or .readonly? == "false"
        or .readonly? == 1
        or .readonly? == "1"
        or .readonly? == true
        or .readonly? == "true"
      )
    )
  ' \
  "${response_file}" >/dev/null; then
  printf 'Calendar delegate inventory is incomplete\n' >&2
  exit 1
fi
calendar_delegate_count="$(jq -r '.data | length' "${response_file}")"
calendar_test_delegate_count="$(
  jq -r \
    '
      [
        .data[]
        | select(
            (.delegator | split("@")[0] | startswith("tfcpanelcal"))
            or (.delegatee | split("@")[0] | startswith("tfcpanelcal"))
          )
      ]
      | length
    ' \
    "${response_file}"
)"

request \
  'execute/Email/list_pops_with_disk?skip_main=1&no_disk=1&get_restrictions=1' \
  'Email account check'
email_account_count="$(jq -r '.data | length' "${response_file}")"
email_test_account_count="$(
  jq -r \
    '[.data[].email | select(startswith("tfcpanel"))] | length' \
    "${response_file}"
)"
email_suspended_count="$(
  jq -r '[.data[] | select(.has_suspended == 1)] | length' "${response_file}"
)"
email_test_suspended_count="$(
  jq -r \
    '[.data[]
      | select(.email | startswith("tfcpanel"))
      | select(.has_suspended == 1)] | length' \
    "${response_file}"
)"
email_accounts="$(jq -r '.data[].email' "${response_file}")"
email_filter_count=0
email_test_filter_count=0
while IFS= read -r address; do
  if [[ -z "${address}" ]]; then
    continue
  fi

  request \
    "execute/Email/list_filters?account=${address}" \
    "Email filter check for ${address}"
  if ! jq -e \
    '
      (.data | type == "array")
      and all(
        .data[];
        (.filtername? | type) == "string"
        and (.rules? | type) == "array"
        and (.actions? | type) == "array"
        and (
          .enabled? == 0
          or .enabled? == "0"
          or .enabled? == false
          or .enabled? == "false"
          or .enabled? == 1
          or .enabled? == "1"
          or .enabled? == true
          or .enabled? == "true"
        )
      )
    ' \
    "${response_file}" >/dev/null; then
    printf 'Email filter inventory is incomplete for %s\n' \
      "${address}" >&2
    exit 1
  fi
  account_filter_count="$(jq -r '.data | length' "${response_file}")"
  account_test_filter_count="$(
    jq -r \
      '[.data[].filtername
        | select(startswith("tfcpanelfilter"))] | length' \
      "${response_file}"
  )"
  email_filter_count=$((email_filter_count + account_filter_count))
  email_test_filter_count=$((email_test_filter_count + account_test_filter_count))
done <<<"${email_accounts}"

request 'execute/Email/list_mail_domains' 'Email forwarder domain inventory'
mail_domains="$(jq -r '.data[].domain' "${response_file}")"
email_forwarder_count=0
email_test_forwarder_count=0
email_auto_responder_count=0
email_test_auto_responder_count=0
while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi

  request \
    "execute/Email/list_forwarders?domain=${domain}" \
    "Email forwarder check for ${domain}"
  domain_forwarder_count="$(jq -r '.data | length' "${response_file}")"
  domain_test_forwarder_count="$(
    jq -r \
      '[.data[].dest | select(startswith("tfcpanelfwd"))] | length' \
      "${response_file}"
  )"
  email_forwarder_count=$((email_forwarder_count + domain_forwarder_count))
  email_test_forwarder_count=$((email_test_forwarder_count + domain_test_forwarder_count))

  request \
    "execute/Email/list_auto_responders?domain=${domain}" \
    "Email autoresponder check for ${domain}"
  domain_auto_responder_count="$(jq -r '.data | length' "${response_file}")"
  domain_test_auto_responder_count="$(
    jq -r \
      '[.data[].email | select(startswith("tfcpanelauto"))] | length' \
      "${response_file}"
  )"
  email_auto_responder_count=$((email_auto_responder_count + domain_auto_responder_count))
  email_test_auto_responder_count=$((email_test_auto_responder_count + domain_test_auto_responder_count))
done <<<"${mail_domains}"

request 'execute/Email/list_domain_forwarders' 'Email domain forwarder check'
email_domain_forwarder_count="$(jq -r '.data | length' "${response_file}")"
email_test_domain_forwarder_count="$(
  jq -r \
    '[.data[].forward | select(startswith("tfcpaneldomainfwd"))] | length' \
    "${response_file}"
)"

request 'execute/Ftp/list_ftp_with_disk?include_acct_types=sub' 'FTP account check'
ftp_account_count="$(jq -r '.data | length' "${response_file}")"
ftp_test_account_count="$(
  jq -r \
    '[.data[].login | select(startswith("tfcpanelftp"))] | length' \
  "${response_file}"
)"

request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=DenyIp&cpanel_jsonapi_func=listdenyips" \
  'IP block check'
ip_block_count="$(jq -r '.cpanelresult.data | length' "${response_file}")"
ip_test_block_count="$(
  jq -r \
    '[.cpanelresult.data[]
      | select(
          .ip == "198.51.100.253"
          or .ip == "198.51.100.254"
          or .ip == "198.51.100.240-198.51.100.242"
          or .ip == "198.51.100.240/31"
          or .ip == "198.51.100.242"
          or .ip == "203.0.113.248/30"
          or (.ip | startswith("2001:0db8:ffff:"))
        )] | length' \
    "${response_file}"
)"

request 'execute/DomainInfo/list_domains' 'DNS zone inventory'
dns_zones="$(
  jq -r \
    '[.data.main_domain] + (.data.addon_domains // []) | .[]' \
    "${response_file}"
)"
dns_record_count=0
dns_test_record_count=0
while IFS= read -r zone; do
  if [[ -z "${zone}" ]]; then
    continue
  fi

  request "execute/DNS/parse_zone?zone=${zone}" "DNS record check for ${zone}"
  zone_record_count="$(
    jq -r \
      '[.data[] | select(.record_type != null)] | length' \
      "${response_file}"
  )"
  zone_test_record_count="$(
    jq -r \
      '[.data[]
        | select(.record_type != null)
        | select(
            (try (.dname_b64 | @base64d) catch "")
            | startswith("tfcpaneldns")
          )] | length' \
      "${response_file}"
  )"
  dns_record_count=$((dns_record_count + zone_record_count))
  dns_test_record_count=$((dns_test_record_count + zone_test_record_count))
done <<<"${dns_zones}"

request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=AddonDomain&cpanel_jsonapi_func=listaddondomains" \
  'addon domain check'
addon_domain_count="$(jq -r '.cpanelresult.data | length' "${response_file}")"
addon_domain_test_count="$(
  jq -r \
    '[.cpanelresult.data[].domain | select(startswith("tfcpaneladdon"))] | length' \
    "${response_file}"
)"

request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Park&cpanel_jsonapi_func=listparkeddomains" \
  'domain alias check'
domain_alias_count="$(jq -r '.cpanelresult.data | length' "${response_file}")"
domain_alias_test_count="$(
  jq -r \
    '[.cpanelresult.data[].domain | select(startswith("tfcpanelalias"))] | length' \
    "${response_file}"
)"

request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=SubDomain&cpanel_jsonapi_func=listsubdomains" \
  'subdomain check'
subdomain_count="$(jq -r '.cpanelresult.data | length' "${response_file}")"
subdomain_test_count="$(
  jq -r \
    '[.cpanelresult.data[].domain | select(startswith("tfcpanelsub"))] | length' \
    "${response_file}"
)"

request 'execute/ModSecurity/list_domains' 'ModSecurity domain check'
modsecurity_domain_count="$(jq -r '.data | length' "${response_file}")"
modsecurity_disabled_count="$(
  jq -r '[.data[] | select(.enabled == 0)] | length' "${response_file}"
)"
modsecurity_test_disabled_count="$(
  jq -r \
    '[.data[]
      | select(.enabled == 0)
      | .domain
      | select(startswith("tfcpanelsubmodsecurity"))] | length' \
    "${response_file}"
)"

request \
  'execute/Fileman/list_files?dir=public_html&show_hidden=1&limit=1000' \
  'test domain directory check'
domain_test_directory_count="$(
  jq -r \
    '[.data[]
      | select(.type == "dir")
      | .file
      | select(startswith("tfcpanel-"))] | length' \
    "${response_file}"
)"

directory_privacy_test_directory_count=0
request \
  'execute/Fileman/list_files?dir=&show_hidden=1&limit=1000' \
  'cPanel home directory check'
git_repository_test_directory_count="$(
  jq -r \
    '[.data[]
      | select(.type == "dir")
      | .file
      | select(
          startswith("tfcpanel-git-")
          or startswith(".terraform-cpanel-git-delete-")
        )] | length' \
    "${response_file}"
)"
home_has_trash="$(
  jq -r \
    '[.data[] | select(.type == "dir" and .file == ".trash")] | length' \
    "${response_file}"
)"
home_has_password_root="$(
  jq -r \
    '[.data[] | select(.type == "dir" and .file == ".htpasswds")] | length' \
    "${response_file}"
)"
git_repository_test_trash_count=0
if [[ "${home_has_trash}" != "0" ]]; then
  request \
    'execute/Fileman/list_files?dir=.trash&show_hidden=1&limit=1000' \
    'cPanel trash check'
  git_repository_test_trash_count="$(
    jq -r \
      '[.data[]
        | select(.type == "dir")
        | .file
        | select(
            startswith("tfcpanel-git-")
            or startswith(".terraform-cpanel-git-delete-")
          )] | length' \
      "${response_file}"
  )"
fi
if [[ "${home_has_password_root}" != "0" ]]; then
  request \
    'execute/Fileman/list_files?dir=.htpasswds&show_hidden=1&limit=1000' \
    'Directory Privacy password root check'
  if jq -e \
    '.data[] | select(.type == "dir" and .file == "public_html")' \
    "${response_file}" >/dev/null; then
    request \
      'execute/Fileman/list_files?dir=.htpasswds/public_html&show_hidden=1&limit=1000' \
      'Directory Privacy test directory check'
    directory_privacy_test_directory_count="$(
      jq -r \
        '[.data[]
          | select(.type == "dir")
          | .file
          | select(startswith("tfcpanel-privacy-"))] | length' \
        "${response_file}"
    )"
  fi
fi

request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Cron&cpanel_jsonapi_func=fetchcron" \
  'cron check'
cron_count="$(jq -r '.cpanelresult.data | length' "${response_file}")"
cron_command_count="$(
  jq -r '[.cpanelresult.data[] | select(.type == "command")] | length' "${response_file}"
)"
cron_variable_count="$(
  jq -r '[.cpanelresult.data[] | select(.type == "variable")] | length' "${response_file}"
)"

if [[ "${CPANEL_REQUIRE_EMPTY:-0}" == "1" ]]; then
  if [[
    "${api_test_token_count}" != "0"
    || "${dynamic_dns_test_count}" != "0"
    || "${redirect_test_count}" != "0"
    || "${mime_type_test_count}" != "0"
    || "${apache_handler_test_count}" != "0"
    || "${git_repository_test_count}" != "0"
    || "${passenger_application_test_count}" != "0"
    || "${ssl_certificate_test_count}" != "0"
    || "${database_count}" != "0"
    || "${user_count}" != "0"
    || "${mysql_test_database_count}" != "0"
    || "${mysql_test_user_count}" != "0"
    || "${mysql_test_remote_host_count}" != "0"
    || "${calendar_test_delegate_count}" != "0"
    || "${email_test_account_count}" != "0"
    || "${email_test_suspended_count}" != "0"
    || "${email_test_filter_count}" != "0"
    || "${email_test_forwarder_count}" != "0"
    || "${email_test_domain_forwarder_count}" != "0"
    || "${email_test_auto_responder_count}" != "0"
    || "${ftp_test_account_count}" != "0"
    || "${ip_test_block_count}" != "0"
    || "${dns_test_record_count}" != "0"
    || "${addon_domain_test_count}" != "0"
    || "${domain_alias_test_count}" != "0"
    || "${subdomain_test_count}" != "0"
    || "${modsecurity_test_disabled_count}" != "0"
    || "${domain_test_directory_count}" != "0"
    || "${git_repository_test_directory_count}" != "0"
    || "${git_repository_test_trash_count}" != "0"
    || "${directory_privacy_test_directory_count}" != "0"
    || "${cron_count}" != "0"
  ]]; then
    printf 'cPanel test-managed inventory is not empty\n' >&2
    printf '  test API tokens: %s\n' "${api_test_token_count}" >&2
    printf '  test Dynamic DNS domains: %s\n' \
      "${dynamic_dns_test_count}" >&2
    printf '  test HTTP redirects: %s\n' "${redirect_test_count}" >&2
    printf '  test custom MIME types: %s\n' "${mime_type_test_count}" >&2
    printf '  test Apache handlers: %s\n' \
      "${apache_handler_test_count}" >&2
    printf '  test Git repositories: %s\n' \
      "${git_repository_test_count}" >&2
    printf '  test Passenger applications: %s\n' \
      "${passenger_application_test_count}" >&2
    printf '  test stored SSL certificates: %s\n' \
      "${ssl_certificate_test_count}" >&2
    printf '  PostgreSQL databases: %s\n' "${database_count}" >&2
    printf '  PostgreSQL users: %s\n' "${user_count}" >&2
    printf '  MySQL test databases: %s\n' "${mysql_test_database_count}" >&2
    printf '  MySQL test users: %s\n' "${mysql_test_user_count}" >&2
    printf '  test remote MySQL hosts: %s\n' \
      "${mysql_test_remote_host_count}" >&2
    printf '  test calendar delegates: %s\n' \
      "${calendar_test_delegate_count}" >&2
    printf '  email test accounts: %s\n' "${email_test_account_count}" >&2
    printf '  suspended email test accounts: %s\n' \
      "${email_test_suspended_count}" >&2
    printf '  test email filters: %s\n' "${email_test_filter_count}" >&2
    printf '  test email forwarders: %s\n' "${email_test_forwarder_count}" >&2
    printf '  test email domain forwarders: %s\n' \
      "${email_test_domain_forwarder_count}" >&2
    printf '  test email autoresponders: %s\n' \
      "${email_test_auto_responder_count}" >&2
    printf '  FTP test accounts: %s\n' "${ftp_test_account_count}" >&2
    printf '  test IP blocks: %s\n' "${ip_test_block_count}" >&2
    printf '  test DNS records: %s\n' "${dns_test_record_count}" >&2
    printf '  test addon domains: %s\n' "${addon_domain_test_count}" >&2
    printf '  test domain aliases: %s\n' "${domain_alias_test_count}" >&2
    printf '  test subdomains: %s\n' "${subdomain_test_count}" >&2
    printf '  disabled test ModSecurity domains: %s\n' \
      "${modsecurity_test_disabled_count}" >&2
    printf '  test domain directories: %s\n' "${domain_test_directory_count}" >&2
    printf '  test Git repository directories: %s\n' \
      "${git_repository_test_directory_count}" >&2
    printf '  test Git trash entries: %s\n' \
      "${git_repository_test_trash_count}" >&2
    printf '  test Directory Privacy password directories: %s\n' \
      "${directory_privacy_test_directory_count}" >&2
    printf '  cron commands: %s\n' "${cron_command_count}" >&2
    printf '  cron variables: %s\n' "${cron_variable_count}" >&2
    exit 1
  fi
fi

printf 'cPanel smoke test passed\n'
printf '  account: %s\n' "${CPANEL_USERNAME}"
printf '  host: %s\n' "${host}"
printf '  version: %s\n' "${cpanel_version}"
printf '  locale: %s (%s, %s; %s available)\n' \
  "${account_locale}" \
  "${account_locale_direction}" \
  "${account_locale_encoding}" \
  "${locale_count}"
printf '  log archival: archive=%s prune=%s retention=%s (effective %s)\n' \
  "${log_archive_logs}" \
  "${log_prune_archive}" \
  "${log_configured_retention}" \
  "${log_effective_retention}"
printf '  API tokens: %s (%s test-managed)\n' \
  "${api_token_count}" \
  "${api_test_token_count}"
printf '  Dynamic DNS domains: %s (%s test-managed)\n' \
  "${dynamic_dns_count}" \
  "${dynamic_dns_test_count}"
printf '  HTTP redirects: %s (%s test-managed)\n' \
  "${redirect_count}" \
  "${redirect_test_count}"
printf '  custom MIME types: %s (%s test-managed)\n' \
  "${mime_type_count}" \
  "${mime_type_test_count}"
printf '  Apache handlers: %s (%s test-managed)\n' \
  "${apache_handler_count}" \
  "${apache_handler_test_count}"
printf '  Git repositories: %s (%s test-managed)\n' \
  "${git_repository_count}" \
  "${git_repository_test_count}"
printf '  Passenger applications: %s (%s test-managed)\n' \
  "${passenger_application_count}" \
  "${passenger_application_test_count}"
printf '  stored SSL certificates: %s (%s test-managed)\n' \
  "${ssl_certificate_count}" \
  "${ssl_certificate_test_count}"
printf '  public_html directory index: %s\n' "${public_html_index_type}"
printf '  public_html directory protected: %s\n' "${public_html_protected}"
printf '  PostgreSQL databases: %s\n' "${database_count}"
printf '  PostgreSQL users: %s\n' "${user_count}"
printf '  MySQL databases: %s (%s test-managed)\n' \
  "${mysql_database_count}" \
  "${mysql_test_database_count}"
printf '  MySQL users: %s (%s test-managed)\n' \
  "${mysql_user_count}" \
  "${mysql_test_user_count}"
printf '  remote MySQL hosts: %s (%s test-managed)\n' \
  "${mysql_remote_host_count}" \
  "${mysql_test_remote_host_count}"
printf '  calendar delegates: %s (%s test-managed)\n' \
  "${calendar_delegate_count}" \
  "${calendar_test_delegate_count}"
printf '  email accounts: %s (%s suspended, %s test-managed, %s test-suspended)\n' \
  "${email_account_count}" \
  "${email_suspended_count}" \
  "${email_test_account_count}" \
  "${email_test_suspended_count}"
printf '  email filters: %s (%s test-managed)\n' \
  "${email_filter_count}" \
  "${email_test_filter_count}"
printf '  email forwarders: %s (%s test-managed)\n' \
  "${email_forwarder_count}" \
  "${email_test_forwarder_count}"
printf '  email domain forwarders: %s (%s test-managed)\n' \
  "${email_domain_forwarder_count}" \
  "${email_test_domain_forwarder_count}"
printf '  email autoresponders: %s (%s test-managed)\n' \
  "${email_auto_responder_count}" \
  "${email_test_auto_responder_count}"
printf '  FTP accounts: %s (%s test-managed)\n' \
  "${ftp_account_count}" \
  "${ftp_test_account_count}"
printf '  IP blocks: %s (%s test-managed)\n' \
  "${ip_block_count}" \
  "${ip_test_block_count}"
printf '  DNS records: %s (%s test-managed)\n' \
  "${dns_record_count}" \
  "${dns_test_record_count}"
printf '  addon domains: %s (%s test-managed)\n' \
  "${addon_domain_count}" \
  "${addon_domain_test_count}"
printf '  domain aliases: %s (%s test-managed)\n' \
  "${domain_alias_count}" \
  "${domain_alias_test_count}"
printf '  subdomains: %s (%s test-managed)\n' \
  "${subdomain_count}" \
  "${subdomain_test_count}"
printf '  ModSecurity domains: %s (%s disabled, %s test-disabled)\n' \
  "${modsecurity_domain_count}" \
  "${modsecurity_disabled_count}" \
  "${modsecurity_test_disabled_count}"
printf '  test domain directories: %s\n' "${domain_test_directory_count}"
printf '  test Git repository directories: %s\n' \
  "${git_repository_test_directory_count}"
printf '  test Git trash entries: %s\n' \
  "${git_repository_test_trash_count}"
printf '  test Directory Privacy password directories: %s\n' \
  "${directory_privacy_test_directory_count}"
printf '  cron commands: %s\n' "${cron_command_count}"
printf '  cron variables: %s\n' "${cron_variable_count}"
