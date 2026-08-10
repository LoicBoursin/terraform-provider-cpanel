#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
tools_directory="${TOOLS_DIR:-${repository_directory}/.git/tools}"

# shellcheck source=tool-versions.sh
source "${script_directory}/tool-versions.sh"

mkdir -p "${tools_directory}"

export GOBIN="${tools_directory}"
GOTOOLCHAIN="$(go env GOVERSION)"
export GOTOOLCHAIN

go install "github.com/rhysd/actionlint/cmd/actionlint@v${actionlint_version}"
go install "golang.org/x/vuln/cmd/govulncheck@v${govulncheck_version}"
go install "github.com/goreleaser/goreleaser/v2@v${goreleaser_version}"

case "$(uname -s)" in
  Darwin)
    tool_os="darwin"
    ;;
  Linux)
    tool_os="linux"
    ;;
  *)
    printf 'Unsupported tool operating system: %s\n' "$(uname -s)" >&2
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
    printf 'Unsupported tool architecture: %s\n' "$(uname -m)" >&2
    exit 1
    ;;
esac

temporary_directory="$(mktemp -d)"
cleanup() {
  rm -rf "${temporary_directory}"
}
trap cleanup EXIT

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

verify_checksum() {
  local archive_path="$1"
  local expected_checksum="$2"
  local archive_name
  archive_name="$(basename "${archive_path}")"

  if [[ -z "${expected_checksum}" ]]; then
    printf 'Pinned checksum not found for %s\n' "${archive_name}" >&2
    exit 1
  fi
  if [[ "$(sha256_file "${archive_path}")" != "${expected_checksum}" ]]; then
    printf 'Checksum mismatch for %s\n' "${archive_name}" >&2
    exit 1
  fi
}

download_release() {
  local url="$1"
  local output_path="$2"

  curl \
    --fail \
    --location \
    --silent \
    --show-error \
    --output "${output_path}" \
    "${url}"
}

golangci_lint_archive="golangci-lint-${golangci_lint_version}-${tool_os}-${tool_arch}.tar.gz"
golangci_lint_release_url="https://github.com/golangci/golangci-lint/releases/download/v${golangci_lint_version}"

download_release \
  "${golangci_lint_release_url}/${golangci_lint_archive}" \
  "${temporary_directory}/${golangci_lint_archive}"

checksum_variable="golangci_lint_checksum_${tool_os}_${tool_arch}"
expected_checksum="${!checksum_variable:-}"
verify_checksum \
  "${temporary_directory}/${golangci_lint_archive}" \
  "${expected_checksum}"

tar -xzf "${temporary_directory}/${golangci_lint_archive}" \
  -C "${temporary_directory}"
install \
  -m 0755 \
  "${temporary_directory}/golangci-lint-${golangci_lint_version}-${tool_os}-${tool_arch}/golangci-lint" \
  "${tools_directory}/golangci-lint"

shellcheck_arch="x86_64"
gitleaks_arch="x64"
if [[ "${tool_arch}" == "arm64" ]]; then
  shellcheck_arch="aarch64"
  gitleaks_arch="arm64"
fi

shellcheck_archive="shellcheck-v${shellcheck_version}.${tool_os}.${shellcheck_arch}.tar.xz"
shellcheck_release_url="https://github.com/koalaman/shellcheck/releases/download/v${shellcheck_version}"
download_release \
  "${shellcheck_release_url}/${shellcheck_archive}" \
  "${temporary_directory}/${shellcheck_archive}"
checksum_variable="shellcheck_checksum_${tool_os}_${tool_arch}"
verify_checksum \
  "${temporary_directory}/${shellcheck_archive}" \
  "${!checksum_variable:-}"
tar -xJf "${temporary_directory}/${shellcheck_archive}" \
  -C "${temporary_directory}"
install \
  -m 0755 \
  "${temporary_directory}/shellcheck-v${shellcheck_version}/shellcheck" \
  "${tools_directory}/shellcheck"

gitleaks_archive="gitleaks_${gitleaks_version}_${tool_os}_${gitleaks_arch}.tar.gz"
gitleaks_release_url="https://github.com/gitleaks/gitleaks/releases/download/v${gitleaks_version}"
download_release \
  "${gitleaks_release_url}/${gitleaks_archive}" \
  "${temporary_directory}/${gitleaks_archive}"
checksum_variable="gitleaks_checksum_${tool_os}_${tool_arch}"
verify_checksum \
  "${temporary_directory}/${gitleaks_archive}" \
  "${!checksum_variable:-}"
tar -xzf "${temporary_directory}/${gitleaks_archive}" \
  -C "${temporary_directory}" \
  gitleaks
install \
  -m 0755 \
  "${temporary_directory}/gitleaks" \
  "${tools_directory}/gitleaks"

syft_archive="syft_${syft_version}_${tool_os}_${tool_arch}.tar.gz"
syft_release_url="https://github.com/anchore/syft/releases/download/v${syft_version}"

download_release \
  "${syft_release_url}/${syft_archive}" \
  "${temporary_directory}/${syft_archive}"

checksum_variable="syft_checksum_${tool_os}_${tool_arch}"
expected_checksum="${!checksum_variable:-}"
verify_checksum \
  "${temporary_directory}/${syft_archive}" \
  "${expected_checksum}"

tar -xzf "${temporary_directory}/${syft_archive}" \
  -C "${tools_directory}" \
  syft

TOOLS_DIR="${tools_directory}" "${script_directory}/check-tools.sh" --release
