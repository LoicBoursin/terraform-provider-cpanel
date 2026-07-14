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

get_request() {
  local endpoint="$1"
  local label="$2"
  local http_code
  local status

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

uapi_post() {
  local module="$1"
  local function="$2"
  local label="$3"
  local http_code
  local parameter
  local status
  local curl_arguments=(
    --silent
    --show-error
    --max-time 90
    --request POST
    --output "${response_file}"
    --write-out '%{http_code}'
    --header "${authorization}"
    --header 'Content-Type: application/x-www-form-urlencoded'
  )

  shift 3
  for parameter in "$@"; do
    curl_arguments+=(--data-urlencode "${parameter}")
  done

  http_code="$(curl "${curl_arguments[@]}" "${host}/execute/${module}/${function}")"

  if [[ "${http_code}" != "200" ]]; then
    printf '%s failed with HTTP %s\n' "${label}" "${http_code}" >&2
    exit 1
  fi

  status="$(jq -r '.status // empty' "${response_file}")"
  if [[ "${status}" != "1" ]]; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{errors, messages}' "${response_file}")" >&2
    exit 1
  fi
}

api2_post() {
  local module="$1"
  local function="$2"
  local label="$3"
  local http_code
  local parameter
  local status
  local curl_arguments=(
    --silent
    --show-error
    --max-time 90
    --request POST
    --output "${response_file}"
    --write-out '%{http_code}'
    --header "${authorization}"
    --header 'Content-Type: application/x-www-form-urlencoded'
    --data-urlencode 'cpanel_jsonapi_apiversion=2'
    --data-urlencode "cpanel_jsonapi_user=${CPANEL_USERNAME}"
    --data-urlencode "cpanel_jsonapi_module=${module}"
    --data-urlencode "cpanel_jsonapi_func=${function}"
  )

  shift 3
  for parameter in "$@"; do
    curl_arguments+=(--data-urlencode "${parameter}")
  done

  http_code="$(curl "${curl_arguments[@]}" "${host}/json-api/cpanel")"

  if [[ "${http_code}" != "200" ]]; then
    printf '%s failed with HTTP %s\n' "${label}" "${http_code}" >&2
    exit 1
  fi

  status="$(jq -r '.cpanelresult.event.result // empty' "${response_file}")"
  if [[ "${status}" != "1" ]]; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{cpanelresult}' "${response_file}")" >&2
    exit 1
  fi
}

remove_cron_line() {
  local linekey="$1"
  local label="$2"
  local http_code
  local status

  http_code="$(
    curl \
      --silent \
      --show-error \
      --max-time 90 \
      --request POST \
      --output "${response_file}" \
      --write-out '%{http_code}' \
      --header "${authorization}" \
      --header 'Content-Type: application/x-www-form-urlencoded' \
      --data-urlencode 'cpanel_jsonapi_apiversion=2' \
      --data-urlencode "cpanel_jsonapi_user=${CPANEL_USERNAME}" \
      --data-urlencode 'cpanel_jsonapi_module=Cron' \
      --data-urlencode 'cpanel_jsonapi_func=remove_line' \
      --data-urlencode "linekey=${linekey}" \
      "${host}/json-api/cpanel"
  )"

  if [[ "${http_code}" != "200" ]]; then
    printf '%s failed with HTTP %s\n' "${label}" "${http_code}" >&2
    exit 1
  fi

  status="$(jq -r '.cpanelresult.event.result // empty' "${response_file}")"
  if [[ "${status}" != "1" ]]; then
    printf '%s failed: %s\n' \
      "${label}" \
      "$(jq -c '{cpanelresult}' "${response_file}")" >&2
    exit 1
  fi
}

test_prefix="${CPANEL_USERNAME}_tf"
deleted_databases=0
deleted_users=0
deleted_mysql_databases=0
deleted_mysql_users=0
deleted_email_accounts=0
deleted_email_forwarders=0
deleted_email_domain_forwarders=0
deleted_email_auto_responders=0
deleted_ftp_accounts=0
deleted_ip_blocks=0
deleted_dns_records=0
deleted_addon_domains=0
deleted_domain_aliases=0
deleted_subdomains=0
deleted_domain_directories=0
deleted_cron_lines=0

get_request 'execute/Postgresql/list_databases' 'PostgreSQL database inventory'
while IFS= read -r database; do
  if [[ -z "${database}" ]]; then
    continue
  fi
  uapi_post 'Postgresql' 'delete_database' "Delete test database ${database}" "name=${database}"
  deleted_databases=$((deleted_databases + 1))
done < <(
  jq -r \
    --arg prefix "${test_prefix}" \
    '.data[].database | select(startswith($prefix))' \
    "${response_file}"
)

get_request 'execute/Postgresql/list_users' 'PostgreSQL user inventory'
while IFS= read -r user; do
  if [[ -z "${user}" ]]; then
    continue
  fi
  uapi_post 'Postgresql' 'delete_user' "Delete test user ${user}" "name=${user}"
  deleted_users=$((deleted_users + 1))
done < <(
  jq -r \
    --arg prefix "${test_prefix}" \
    '.data[] | select(startswith($prefix))' \
    "${response_file}"
)

get_request 'execute/Mysql/list_databases' 'MySQL database inventory'
while IFS= read -r database; do
  if [[ -z "${database}" ]]; then
    continue
  fi
  uapi_post 'Mysql' 'delete_database' "Delete test MySQL database ${database}" "name=${database}"
  deleted_mysql_databases=$((deleted_mysql_databases + 1))
done < <(
  jq -r \
    --arg prefix "${test_prefix}" \
    '.data[].database | select(startswith($prefix))' \
    "${response_file}"
)

get_request 'execute/Mysql/list_users' 'MySQL user inventory'
while IFS= read -r user; do
  if [[ -z "${user}" ]]; then
    continue
  fi
  uapi_post 'Mysql' 'delete_user' "Delete test MySQL user ${user}" "name=${user}"
  deleted_mysql_users=$((deleted_mysql_users + 1))
done < <(
  jq -r \
    --arg prefix "${test_prefix}" \
    '.data[].user | select(startswith($prefix))' \
    "${response_file}"
)

get_request 'execute/Email/list_pops?skip_main=1' 'Email account inventory'
while IFS= read -r address; do
  if [[ -z "${address}" ]]; then
    continue
  fi
  uapi_post 'Email' 'delete_pop' "Delete test email account ${address}" "email=${address}"
  deleted_email_accounts=$((deleted_email_accounts + 1))
done < <(
  jq -r \
    '.data[].email | select(startswith("tfcpanel"))' \
    "${response_file}"
)

get_request 'execute/Email/list_mail_domains' 'Email forwarder domain inventory'
mail_domains="$(jq -r '.data[].domain' "${response_file}")"
while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi

  get_request \
    "execute/Email/list_forwarders?domain=${domain}" \
    "Email forwarder inventory for ${domain}"
  while IFS=$'\t' read -r address destination; do
    if [[ -z "${address}" || -z "${destination}" ]]; then
      continue
    fi
    uapi_post \
      'Email' \
      'delete_forwarder' \
      "Delete test email forwarder ${address}" \
      "address=${address}" \
      "forwarder=${destination}"
    deleted_email_forwarders=$((deleted_email_forwarders + 1))
  done < <(
    jq -r \
      '.data[]
        | select(.dest | startswith("tfcpanelfwd"))
        | [.dest, .forward]
        | @tsv' \
      "${response_file}"
  )
done <<<"${mail_domains}"

while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi

  get_request \
    "execute/Email/list_auto_responders?domain=${domain}" \
    "Email autoresponder inventory for ${domain}"
  while IFS= read -r address; do
    if [[ -z "${address}" ]]; then
      continue
    fi
    uapi_post \
      'Email' \
      'delete_auto_responder' \
      "Delete test email autoresponder ${address}" \
      "email=${address}"
    deleted_email_auto_responders=$((deleted_email_auto_responders + 1))
  done < <(
    jq -r \
      '.data[].email | select(startswith("tfcpanelauto"))' \
      "${response_file}"
  )
done <<<"${mail_domains}"

get_request 'execute/Email/list_domain_forwarders' 'Email domain forwarder inventory'
while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi
  uapi_post \
    'Email' \
    'delete_domain_forwarder' \
    "Delete test email domain forwarder ${domain}" \
    "domain=${domain}"
  deleted_email_domain_forwarders=$((deleted_email_domain_forwarders + 1))
done < <(
  jq -r \
    '.data[]
      | select(.forward | startswith("tfcpaneldomainfwd"))
      | .dest' \
    "${response_file}"
)

get_request 'execute/Ftp/list_ftp_with_disk?include_acct_types=sub' 'FTP account inventory'
while IFS= read -r login; do
  if [[ -z "${login}" ]]; then
    continue
  fi
  user="${login%@*}"
  domain="${login#*@}"
  uapi_post \
    'Ftp' \
    'delete_ftp' \
    "Delete test FTP account ${login}" \
    "user=${user}" \
    "domain=${domain}" \
    'destroy=1'
  deleted_ftp_accounts=$((deleted_ftp_accounts + 1))
done < <(
  jq -r \
    '.data[].login | select(startswith("tfcpanelftp"))' \
    "${response_file}"
)

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=DenyIp&cpanel_jsonapi_func=listdenyips" \
  'IP block inventory'
while IFS= read -r address; do
  if [[ -z "${address}" ]]; then
    continue
  fi
  uapi_post \
    'BlockIP' \
    'remove_ip' \
    "Delete test IP block ${address}" \
    "ip=${address}"
  deleted_ip_blocks=$((deleted_ip_blocks + 1))
done < <(
  jq -r \
    '.cpanelresult.data[]
      | select(
          .ip == "198.51.100.253"
          or .ip == "198.51.100.254"
          or .ip == "198.51.100.240-198.51.100.242"
          or .ip == "198.51.100.240/31"
          or .ip == "198.51.100.242"
          or .ip == "203.0.113.248/30"
          or (.ip | startswith("2001:0db8:ffff:"))
        )
      | .ip' \
    "${response_file}"
)

get_request 'execute/DomainInfo/list_domains' 'DNS zone inventory'
dns_zones="$(
  jq -r \
    '[.data.main_domain] + (.data.addon_domains // []) | .[]' \
    "${response_file}"
)"
while IFS= read -r zone; do
  if [[ -z "${zone}" ]]; then
    continue
  fi

  while true; do
    get_request "execute/DNS/parse_zone?zone=${zone}" "DNS record inventory for ${zone}"
    line_index="$(
      jq -r \
        'first(
          .data[]
          | select(.record_type != null)
          | select(
              (try (.dname_b64 | @base64d) catch "")
              | startswith("tfcpaneldns")
            )
          | .line_index
        ) // empty' \
        "${response_file}"
    )"
    if [[ -z "${line_index}" ]]; then
      break
    fi

    serial="$(
      jq -r \
        '.data[]
          | select(.record_type == "SOA")
          | .data_b64[2]
          | @base64d' \
        "${response_file}"
    )"
    if [[ -z "${serial}" ]]; then
      printf 'DNS zone %s does not contain a readable SOA serial\n' "${zone}" >&2
      exit 1
    fi

    mutation_label="Delete test DNS record at line ${line_index} in ${zone}"
    uapi_post \
      'DNS' \
      'mass_edit_zone' \
      "${mutation_label}" \
      "zone=${zone}" \
      "serial=${serial}" \
      "remove=${line_index}"
    deleted_dns_records=$((deleted_dns_records + 1))
  done
done <<<"${dns_zones}"

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Park&cpanel_jsonapi_func=listparkeddomains" \
  'Domain alias inventory'
while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi
  api2_post \
    'Park' \
    'unpark' \
    "Delete test domain alias ${domain}" \
    "domain=${domain}"
  deleted_domain_aliases=$((deleted_domain_aliases + 1))
done < <(
  jq -r \
    '.cpanelresult.data[].domain | select(startswith("tfcpanelalias"))' \
    "${response_file}"
)

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=SubDomain&cpanel_jsonapi_func=listsubdomains" \
  'Subdomain inventory'
test_subdomains="$(
  jq -r \
    '.cpanelresult.data[].domain | select(startswith("tfcpanelsub"))' \
    "${response_file}"
)"
while IFS= read -r domain; do
  if [[ -z "${domain}" ]]; then
    continue
  fi
  api2_post \
    'SubDomain' \
    'delsubdomain' \
    "Delete test subdomain ${domain}" \
    "domain=${domain}"
  deleted_subdomains=$((deleted_subdomains + 1))
done <<<"${test_subdomains}"

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=AddonDomain&cpanel_jsonapi_func=listaddondomains" \
  'Addon domain inventory'
test_addon_domains="$(
  jq -r \
    '.cpanelresult.data[]
      | select(.domain | startswith("tfcpaneladdon"))
      | [.domain, .domainkey]
      | @tsv' \
    "${response_file}"
)"
while IFS=$'\t' read -r domain domain_key; do
  if [[ -z "${domain}" || -z "${domain_key}" ]]; then
    continue
  fi
  api2_post \
    'AddonDomain' \
    'deladdondomain' \
    "Delete test addon domain ${domain}" \
    "domain=${domain}" \
    "subdomain=${domain_key}"
  deleted_addon_domains=$((deleted_addon_domains + 1))
done <<<"${test_addon_domains}"

get_request \
  'execute/Fileman/list_files?dir=public_html&show_hidden=1&limit=1000' \
  'Test domain directory inventory'
test_domain_directories="$(
  jq -r \
    '.data[]
      | select(.type == "dir")
      | .file
      | select(startswith("tfcpanel-"))' \
    "${response_file}"
)"
while IFS= read -r directory; do
  if [[ -z "${directory}" ]]; then
    continue
  fi
  api2_post \
    'Fileman' \
    'fileop' \
    "Delete test domain directory public_html/${directory}" \
    'op=unlink' \
    "sourcefiles=public_html/${directory}" \
    'doubledecode=0'
  deleted_domain_directories=$((deleted_domain_directories + 1))
done <<<"${test_domain_directories}"

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Cron&cpanel_jsonapi_func=fetchcron" \
  'Cron inventory'
while IFS= read -r linekey; do
  if [[ -z "${linekey}" ]]; then
    continue
  fi
  remove_cron_line "${linekey}" "Delete test cron line ${linekey}"
  deleted_cron_lines=$((deleted_cron_lines + 1))
done < <(
  jq -r \
    '.cpanelresult.data[]
      | select(.type == "command")
      | select((.command // "") | contains("# terraform-provider-cpanel-"))
      | .linekey' \
    "${response_file}"
)

get_request \
  "json-api/cpanel?cpanel_jsonapi_user=${CPANEL_USERNAME}&cpanel_jsonapi_apiversion=2&cpanel_jsonapi_module=Cron&cpanel_jsonapi_func=fetchcron" \
  'Cron inventory after test cleanup'
cron_command_count="$(
  jq -r '[.cpanelresult.data[] | select(.type == "command")] | length' "${response_file}"
)"

if [[ "${cron_command_count}" == "0" ]]; then
  while IFS= read -r linekey; do
    if [[ -z "${linekey}" ]]; then
      continue
    fi
    remove_cron_line "${linekey}" "Delete generated cron variable ${linekey}"
    deleted_cron_lines=$((deleted_cron_lines + 1))
  done < <(
    jq -r \
      '.cpanelresult.data[]
        | select(.type == "variable")
        | select(
            (.key == "MAILTO" and (.value // "") == "")
            or (.key == "SHELL" and (.value // "") == "/bin/bash")
          )
        | .linekey' \
      "${response_file}"
  )
fi

printf 'cPanel test cleanup passed\n'
printf '  PostgreSQL databases deleted: %d\n' "${deleted_databases}"
printf '  PostgreSQL users deleted: %d\n' "${deleted_users}"
printf '  MySQL databases deleted: %d\n' "${deleted_mysql_databases}"
printf '  MySQL users deleted: %d\n' "${deleted_mysql_users}"
printf '  email accounts deleted: %d\n' "${deleted_email_accounts}"
printf '  email forwarders deleted: %d\n' "${deleted_email_forwarders}"
printf '  email domain forwarders deleted: %d\n' \
  "${deleted_email_domain_forwarders}"
printf '  email autoresponders deleted: %d\n' \
  "${deleted_email_auto_responders}"
printf '  FTP accounts deleted: %d\n' "${deleted_ftp_accounts}"
printf '  IP blocks deleted: %d\n' "${deleted_ip_blocks}"
printf '  DNS records deleted: %d\n' "${deleted_dns_records}"
printf '  addon domains deleted: %d\n' "${deleted_addon_domains}"
printf '  domain aliases deleted: %d\n' "${deleted_domain_aliases}"
printf '  subdomains deleted: %d\n' "${deleted_subdomains}"
printf '  test domain directories deleted: %d\n' "${deleted_domain_directories}"
printf '  cron lines deleted: %d\n' "${deleted_cron_lines}"
