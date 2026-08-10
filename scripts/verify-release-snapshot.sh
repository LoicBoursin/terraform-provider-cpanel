#!/usr/bin/env bash

set -euo pipefail

script_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repository_directory="$(cd "${script_directory}/.." && pwd)"
distribution_directory="${DIST_DIR:-${repository_directory}/dist}"
source_manifest="${repository_directory}/terraform-registry-manifest.json"
expected_release_name="${EXPECTED_RELEASE_NAME:-}"

if [[ ! -d "${distribution_directory}" ]]; then
  printf 'Missing release distribution directory: %s\n' \
    "${distribution_directory}" >&2
  exit 1
fi
if [[ ! -f "${source_manifest}" ]]; then
  printf 'Missing Terraform Registry manifest: %s\n' \
    "${source_manifest}" >&2
  exit 1
fi
if ! jq -e \
  '.version == 1 and .metadata.protocol_versions == ["6.0"]' \
  "${source_manifest}" >/dev/null; then
  printf 'Invalid Terraform Registry manifest: %s\n' \
    "${source_manifest}" >&2
  exit 1
fi

checksum_files=("${distribution_directory}"/*_SHA256SUMS)
if [[ ! -e "${checksum_files[0]}" || "${#checksum_files[@]}" != "1" ]]; then
  printf 'Expected exactly one release checksum file in %s\n' \
    "${distribution_directory}" >&2
  exit 1
fi

checksum_file="${checksum_files[0]}"
release_prefix="${checksum_file%_SHA256SUMS}"
release_name="$(basename "${release_prefix}")"
if [[ "${release_name}" != terraform-provider-cpanel_* ]]; then
  printf 'Invalid release project prefix: %s\n' "${release_name}" >&2
  exit 1
fi
if [[ -n "${expected_release_name}" &&
  "${release_name}" != "${expected_release_name}" ]]; then
  printf 'Release name %s does not match expected release %s\n' \
    "${release_name}" \
    "${expected_release_name}" >&2
  exit 1
fi
release_version="${release_name#terraform-provider-cpanel_}"
if [[ -z "${release_version}" ]]; then
  printf 'Release version is missing from %s\n' "${release_name}" >&2
  exit 1
fi
release_manifest="${release_prefix}_manifest.json"
if [[ ! -f "${release_manifest}" ]]; then
  printf 'Release Registry manifest is missing: %s\n' \
    "${release_manifest}" >&2
  exit 1
fi
if ! cmp -s "${source_manifest}" "${release_manifest}"; then
  printf 'Release Registry manifest does not match the source manifest\n' >&2
  exit 1
fi

targets=(
  darwin_amd64
  darwin_arm64
  freebsd_386
  freebsd_amd64
  freebsd_arm
  freebsd_arm64
  linux_386
  linux_amd64
  linux_arm
  linux_arm64
  windows_386
  windows_amd64
  windows_arm64
)
expected_archive_names=()
expected_sbom_names=()
expected_checksum_names=()
for target in "${targets[@]}"; do
  archive_name="${release_name}_${target}.zip"
  sbom_name="${archive_name}.sbom.json"
  expected_archive_names+=("${archive_name}")
  expected_sbom_names+=("${sbom_name}")
  expected_checksum_names+=("${archive_name}" "${sbom_name}")
done
expected_checksum_names+=("$(basename "${release_manifest}")")

shopt -s nullglob
archives=("${distribution_directory}"/*.zip)
sboms=("${distribution_directory}"/*.sbom.json)
actual_archive_names="$({
  for archive in "${archives[@]}"; do
    basename "${archive}"
  done
} | LC_ALL=C sort)"
actual_sbom_names="$({
  for sbom in "${sboms[@]}"; do
    basename "${sbom}"
  done
} | LC_ALL=C sort)"
expected_archive_inventory="$(printf '%s\n' "${expected_archive_names[@]}" | LC_ALL=C sort)"
expected_sbom_inventory="$(printf '%s\n' "${expected_sbom_names[@]}" | LC_ALL=C sort)"
if [[ "${actual_archive_names}" != "${expected_archive_inventory}" ]]; then
  printf 'Release archive inventory does not match the supported targets\n' >&2
  exit 1
fi
if [[ "${actual_sbom_names}" != "${expected_sbom_inventory}" ]]; then
  printf 'Release SBOM inventory does not match the supported targets\n' >&2
  exit 1
fi

for sbom in "${sboms[@]}"; do
  expected_sbom_name="$(basename "${sbom%.sbom.json}")"
  if ! jq -e \
    --arg expected_name "${expected_sbom_name}" \
    '
      .spdxVersion == "SPDX-2.3"
      and .dataLicense == "CC0-1.0"
      and .SPDXID == "SPDXRef-DOCUMENT"
      and .name == $expected_name
      and (
        (.documentNamespace | type) == "string"
        and (.documentNamespace | test("^https?://"))
      )
      and (
        (.creationInfo.created | type) == "string"
        and (.creationInfo.created | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$"))
      )
      and (
        (.creationInfo.creators | type) == "array"
        and (.creationInfo.creators | length) > 0
        and all(.creationInfo.creators[]; type == "string" and length > 0)
      )
      and (.packages | type) == "array"
      and (.packages | length) > 0
      and all(
        .packages[];
        ((.name | type) == "string" and (.name | length) > 0)
        and (
          (.SPDXID | type) == "string"
          and (.SPDXID | test("^SPDXRef-[A-Za-z0-9.-]+$"))
        )
        and (
          (.downloadLocation | type) == "string"
          and (.downloadLocation | length) > 0
        )
        and (.filesAnalyzed | type) == "boolean"
        and (
          (.licenseConcluded | type) == "string"
          and (.licenseConcluded | length) > 0
        )
        and (
          (.licenseDeclared | type) == "string"
          and (.licenseDeclared | length) > 0
        )
        and (
          (.copyrightText | type) == "string"
          and (.copyrightText | length) > 0
        )
        and (
          (has("checksums") | not)
          or (
            (.checksums | type) == "array"
            and (.checksums | length) > 0
            and all(
              .checksums[];
              ((.algorithm | type) == "string" and (.algorithm | length) > 0)
              and (
                (.checksumValue | type) == "string"
                and (.checksumValue | test("^[0-9A-Fa-f]+$"))
              )
            )
          )
        )
      )
      and (.files | type) == "array"
      and (.files | length) > 0
      and all(
        .files[];
        ((.fileName | type) == "string" and (.fileName | length) > 0)
        and (
          (.SPDXID | type) == "string"
          and (.SPDXID | test("^SPDXRef-[A-Za-z0-9.-]+$"))
        )
        and (.fileTypes | type) == "array"
        and (.fileTypes | length) > 0
        and all(
          .fileTypes[];
          type == "string" and test("^[A-Z][A-Z0-9_]*$")
        )
        and (.checksums | type) == "array"
        and (.checksums | length) > 0
        and all(
          .checksums[];
          ((.algorithm | type) == "string" and (.algorithm | length) > 0)
          and (
            (.checksumValue | type) == "string"
            and (.checksumValue | test("^[0-9A-Fa-f]+$"))
          )
        )
        and (
          (.licenseConcluded | type) == "string"
          and (.licenseConcluded | length) > 0
        )
        and (.licenseInfoInFiles | type) == "array"
        and (.licenseInfoInFiles | length) > 0
        and all(
          .licenseInfoInFiles[];
          type == "string" and length > 0
        )
        and (
          (.copyrightText | type) == "string"
          and (.copyrightText | length) > 0
        )
      )
      and (
        ([.SPDXID] + [.packages[].SPDXID] + [.files[].SPDXID]) as $element_ids
        | ($element_ids | length) == ($element_ids | unique | length)
        and (.relationships | type) == "array"
        and (.relationships | length) > 0
        and all(
          .relationships[];
          . as $relationship
          | (type == "object")
          and (
            (.spdxElementId | type) == "string"
            and (.spdxElementId | test("^SPDXRef-[A-Za-z0-9.-]+$"))
          )
          and (
            (.relationshipType | type) == "string"
            and (.relationshipType | test("^[A-Z][A-Z0-9_]*$"))
          )
          and (
            (.relatedSpdxElement | type) == "string"
            and (.relatedSpdxElement | test("^SPDXRef-[A-Za-z0-9.-]+$"))
          )
          and (($element_ids | index($relationship.spdxElementId)) != null)
          and (($element_ids | index($relationship.relatedSpdxElement)) != null)
        )
        and any(
          .relationships[];
          .spdxElementId == "SPDXRef-DOCUMENT"
          and .relationshipType == "DESCRIBES"
          and .relatedSpdxElement != "SPDXRef-DOCUMENT"
        )
      )
    ' \
    "${sbom}" >/dev/null; then
    printf 'Release SBOM has an invalid SPDX structure: %s\n' \
      "${sbom}" >&2
    exit 1
  fi
done

for target in "${targets[@]}"; do
  archive="${distribution_directory}/${release_name}_${target}.zip"
  expected_binary="terraform-provider-cpanel_v${release_version}"
  if [[ "${target}" == windows_* ]]; then
    expected_binary+='.exe'
  fi
  archive_entries="$(unzip -Z1 "${archive}")"
  entry_count="$(wc -l <<<"${archive_entries}" | tr -d ' ')"
  if [[ "${entry_count}" != "4" ]] ||
    ! grep -Fxq "${expected_binary}" <<<"${archive_entries}"; then
    printf 'Release archive has unexpected contents: %s\n' \
      "${archive}" >&2
    exit 1
  fi
  for required_entry in CHANGELOG.md LICENSE README.md; do
    if ! grep -Fxq "${required_entry}" <<<"${archive_entries}"; then
      printf 'Release archive is missing %s: %s\n' \
        "${required_entry}" \
        "${archive}" >&2
      exit 1
    fi
  done
done

if ! checksum_names="$(
  awk '
    length($1) == 64 &&
    $1 ~ /^[0-9A-Fa-f]+$/ &&
    NF == 2 &&
    substr($0, 65, 2) == "  " &&
    substr($0, 67) == $2 {
      print $2
      next
    }
    { invalid = 1 }
    END { exit invalid }
  ' "${checksum_file}"
)"; then
  printf 'Release checksum file has an invalid entry format\n' >&2
  exit 1
fi
actual_checksum_inventory="$(printf '%s\n' "${checksum_names}" | LC_ALL=C sort)"
expected_checksum_inventory="$(printf '%s\n' "${expected_checksum_names[@]}" | LC_ALL=C sort)"
if [[ "${actual_checksum_inventory}" != "${expected_checksum_inventory}" ]]; then
  printf 'Release checksum inventory does not match the release artifacts\n' >&2
  exit 1
fi

(
  cd "${distribution_directory}"
  if command -v sha256sum >/dev/null 2>&1; then
    checksum_command=(sha256sum -c)
  else
    checksum_command=(shasum -a 256 -c)
  fi
  if ! "${checksum_command[@]}" \
    "$(basename "${checksum_file}")" >/dev/null; then
    "${checksum_command[@]}" "$(basename "${checksum_file}")"
    exit 1
  fi
)

printf 'Release snapshot verification passed\n'
printf '  archives: %d\n' "${#archives[@]}"
printf '  SBOMs: %d\n' "${#sboms[@]}"
printf '  Registry manifests: 1\n'
printf '  checksum files: 1\n'
