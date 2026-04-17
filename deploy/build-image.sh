#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEMP_DOCKERIGNORE="${ROOT_DIR}/.dockerignore"
SOURCE_DOCKERIGNORE="${ROOT_DIR}/deploy/.dockerignore"
IMAGE_TAG="${OHOCI_IMAGE_TAG:-ohoci:local}"

cleanup() {
  rm -f "${TEMP_DOCKERIGNORE}"
}

trap cleanup EXIT

cp "${SOURCE_DOCKERIGNORE}" "${TEMP_DOCKERIGNORE}"
docker build -f "${ROOT_DIR}/deploy/Dockerfile" -t "${IMAGE_TAG}" "${ROOT_DIR}"
