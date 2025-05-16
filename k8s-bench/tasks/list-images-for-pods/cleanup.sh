#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

NAMESPACE=list-images-for-pods

kubectl delete namespace ${NAMESPACE}
