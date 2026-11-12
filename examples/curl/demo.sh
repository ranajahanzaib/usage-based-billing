#!/bin/sh
set -eu
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
curl --fail --silent --show-error "$BASE_URL/healthz"
printf '\n'
curl --fail --silent --show-error "$BASE_URL/api/customers"
printf '\n'
curl --fail --silent --show-error "$BASE_URL/api/preview?customer_id=startup"
printf '\n'
