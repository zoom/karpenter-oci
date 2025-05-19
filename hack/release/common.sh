#!/usr/bin/env bash
set -euox pipefail

CURRENT_MAJOR_VERSION="0"

snapshot() {
  local commit_sha version helm_chart_version snapshot_repo

  commit_sha="${1}"
  snapshot_repo="${2}/karpenter/snapshot/"
  version="${commit_sha}"
  helm_chart_version="${CURRENT_MAJOR_VERSION}-${commit_sha}"

  echo "Release Type: snapshot
Release Version: ${version}
Commit: ${commit_sha}
Helm Chart Version ${helm_chart_version}"

  authenticatePrivateRepo
  build "${snapshot_repo}" "${version}" "${helm_chart_version}" "${commit_sha}"
}

release() {
  local commit_sha version helm_chart_version

  commit_sha="${1}"
  version="${2}"
  release_repo="${3}/karpenter/karpenter-oci/"
  helm_chart_version="${version}"

  echo "Release Type: stable
Release Version: ${version}
Commit: ${commit_sha}
Helm Chart Version ${helm_chart_version}"

  authenticate
  build "${release_repo}" "${version}" "${helm_chart_version}" "${commit_sha}"
}

authenticate() {
  echo "should login first"
}
authenticatePrivateRepo() {
  echo "should login first"
}

build() {
  local oci_repo version helm_chart_version commit_sha date_epoch build_date img img_repo img_tag img_digest

  oci_repo="${1}"
  version="${2}"
  helm_chart_version="${3}"
  commit_sha="${4}"

  date_epoch="$(dateEpoch)"
  build_date="$(buildDate "${date_epoch}")"
  img="$(GOFLAGS=${GOFLAGS:-} SOURCE_DATE_EPOCH="${date_epoch}" KO_DATA_DATE_EPOCH="${date_epoch}" KO_DOCKER_REPO="${oci_repo}" ko publish -B -t "${version}" ./cmd/controller)"
  img_repo="$(echo "${img}" | cut -d "@" -f 1 | cut -d ":" -f 1)"
  img_tag="$(echo "${img}" | cut -d "@" -f 1 | cut -d ":" -f 2 -s)"
  img_digest="$(echo "${img}" | cut -d "@" -f 2)"
  cosignOciArtifact "${version}" "${commit_sha}" "${build_date}" "${img}"

  yq e -i ".controller.image.repository = \"${img_repo}\"" charts/karpenter/values.yaml
  yq e -i ".controller.image.tag = \"${img_tag}\"" charts/karpenter/values.yaml
  yq e -i ".controller.image.digest = \"${img_digest}\"" charts/karpenter/values.yaml


#  TODO publishHelmChart
}

cosignOciArtifact() {
  local version commit_sha build_date artifact

  version="${1}"
  commit_sha="${2}"
  build_date="${3}"
  artifact="${4}"

  cosign sign --yes -a version="${version}" -a commitSha="${commit_sha}" -a buildDate="${build_date}" "${artifact}"
}

dateEpoch() {
  git log -1 --format='%ct'
}

buildDate() {
  local date_epoch

  date_epoch="${1}"
  if date --version >/dev/null 2>&1; then
    # GNU date (Linux)
    date -u --date="@${date_epoch}" "+%Y-%m-%dT%H:%M:%SZ"
  else
    # BSD date (macOS)
    date -u -r "${date_epoch}" "+%Y-%m-%dT%H:%M:%SZ"
  fi
}