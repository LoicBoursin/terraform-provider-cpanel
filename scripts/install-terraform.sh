#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
tools_directory="${TOOLS_DIR:-${repository_directory}/.git/tools}"

# shellcheck source=tool-versions.sh
source "${script_directory}/tool-versions.sh"

version="${1:-}"
case "${version}" in
  "${terraform_previous_version}")
    checksum_prefix="terraform_previous"
    ;;
  "${terraform_current_version}")
    checksum_prefix="terraform_current"
    ;;
  *)
    printf 'Terraform version must be one of %s or %s\n' \
      "${terraform_previous_version}" \
      "${terraform_current_version}" >&2
    exit 1
    ;;
esac

case "$(uname -s)" in
  Darwin)
    tool_os="darwin"
    ;;
  Linux)
    tool_os="linux"
    ;;
  *)
    printf 'Unsupported Terraform operating system: %s\n' "$(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  arm64 | aarch64)
    tool_arch="arm64"
    ;;
  x86_64 | amd64)
    tool_arch="amd64"
    ;;
  *)
    printf 'Unsupported Terraform architecture: %s\n' "$(uname -m)" >&2
    exit 1
    ;;
esac

checksum_variable="${checksum_prefix}_checksum_${tool_os}_${tool_arch}"
expected_checksum="${!checksum_variable:-}"
if [[ -z "${expected_checksum}" ]]; then
  printf 'Pinned Terraform checksum not found for %s/%s\n' \
    "${tool_os}" \
    "${tool_arch}" >&2
  exit 1
fi

install_directory="${tools_directory}/terraform-${version}"
terraform_path="${install_directory}/terraform"
if [[ -x "${terraform_path}" ]] &&
  [[ "$("${terraform_path}" version -json | jq -r '.terraform_version')" == "${version}" ]]; then
  printf '%s\n' "${install_directory}"
  exit 0
fi

temporary_directory="$(mktemp -d)"
cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

archive="terraform_${version}_${tool_os}_${tool_arch}.zip"
archive_path="${temporary_directory}/${archive}"
curl \
  --fail \
  --location \
  --silent \
  --show-error \
  --output "${archive_path}" \
  "https://releases.hashicorp.com/terraform/${version}/${archive}"

if command -v sha256sum >/dev/null 2>&1; then
  actual_checksum="$(sha256sum "${archive_path}" | awk '{print $1}')"
else
  actual_checksum="$(shasum -a 256 "${archive_path}" | awk '{print $1}')"
fi
if [[ "${actual_checksum}" != "${expected_checksum}" ]]; then
  printf 'Terraform checksum mismatch for %s\n' "${archive}" >&2
  exit 1
fi

mkdir -p "${install_directory}"
unzip -oq "${archive_path}" terraform -d "${temporary_directory}"
install -m 0755 "${temporary_directory}/terraform" "${terraform_path}"

actual_version="$("${terraform_path}" version -json | jq -r '.terraform_version')"
if [[ "${actual_version}" != "${version}" ]]; then
  printf 'Unexpected Terraform version: %s; expected %s\n' \
    "${actual_version}" \
    "${version}" >&2
  exit 1
fi

printf '%s\n' "${install_directory}"
