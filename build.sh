#!/usr/bin/env bash
# Build the current worktree, including uncommitted source and etc/ changes.
set -euo pipefail

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$repo_dir"

usage() {
    cat <<'USAGE'
Usage: ./build.sh [IMAGE] [--push] [--no-cache]

Default image: harbor.zhejianglab.com/zjnc-tech/dcgm-exporter:4.8.4-nhi-v1.1
Builds linux/amd64 from the current worktree; does not fetch or switch branches.
--push       Push after the local build and version check succeed.
--no-cache   Rebuild all layers (source changes invalidate their cache automatically).

Environment:
  IMAGE          Output image, overridden by positional IMAGE
  UBUNTU_IMAGE   Ubuntu 22.04 base (defaults to the Zhejiang Lab registry)
  PULL           true to refresh the base image; default false
USAGE
}

image="${IMAGE:-harbor.zhejianglab.com/zjnc-tech/dcgm-exporter:4.8.4-nhi-v1.1}"
ubuntu_image="${UBUNTU_IMAGE:-cr.registry.res.cloud.zhejianglab.com/infrahi-installer/ubuntu:22.04}"
pull="${PULL:-false}"
push=false
cache_args=()
image_set=false
for arg in "$@"; do
    case "$arg" in
        --help|-h) usage; exit 0 ;;
        --push) push=true ;;
        --no-cache) cache_args=(--no-cache) ;;
        -*) echo "Unknown option: $arg" >&2; usage >&2; exit 2 ;;
        *)
            if [[ "$image_set" == true ]]; then
                echo "Specify only one image name" >&2
                exit 2
            fi
            image="$arg"
            image_set=true
            ;;
    esac
done
case "$pull" in true|false) ;; *) echo "PULL must be true or false" >&2; exit 2 ;; esac

# Read the repository's pinned versions instead of maintaining another version list.
source hack/versions.env
command -v docker >/dev/null
docker buildx version >/dev/null
docker info >/dev/null
# The Dockerfile accepts prepared offline caches; empty directories use downloads.
mkdir -p .go/compiler .go/pkg/mod .workspace
revision="$(git rev-parse HEAD 2>/dev/null || echo unknown)"
if [[ -n "$(git status --porcelain 2>/dev/null)" ]]; then
    revision="${revision}-dirty"
fi
printf 'Building %s from %s (revision %s)\n' "$image" "$repo_dir" "$revision"
docker buildx build \
    --pull="$pull" \
    "${cache_args[@]}" \
    --platform linux/amd64 \
    --target runtime-ubuntu \
    --build-arg "UBUNTU_IMAGE=$ubuntu_image" \
    --build-arg "GOLANG_VERSION=$GO_VERSION" \
    --build-arg "GO_LINUX_AMD64_SHA256=$GO_LINUX_AMD64_SHA256" \
    --build-arg "GO_LINUX_ARM64_SHA256=$GO_LINUX_ARM64_SHA256" \
    --build-arg "DCGM_VERSION=$DCGM_VERSION" \
    --build-arg "VERSION=$EXPORTER_VERSION" \
    --build-arg "SOURCE_REVISION=$revision" \
    --build-arg GOWORK=off \
    --provenance=false \
    --load \
    -f docker/Dockerfile.wxs \
    -t "$image" .

docker run --rm --platform linux/amd64 "$image" --version
if [[ "$push" == true ]]; then
    docker push "$image"
else
    printf 'Built locally: %s\nTo publish: docker push %s\n' "$image" "$image"
fi
