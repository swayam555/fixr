#!/usr/bin/env bash
# Logs in through nginx (HTTP and HTTPS), checks /api/me, and shows which order-service instance served each call.
set -euo pipefail
BASE=${BASE:-http://localhost:8080}
TLS=${TLS:-https://localhost:8443}
CA=infra/generated/certs/ca.pem
PHONE=${PHONE:-+9198$(shuf -i 10000000-99999999 -n1)}
ROLE=${ROLE:-CUSTOMER}
json() { python3 -c "import sys,json;print(json.load(sys.stdin)$1)"; }

R=$(curl -sf "$BASE/api/auth/otp" -H 'Content-Type: application/json' -d "{\"phone\":\"$PHONE\",\"role\":\"$ROLE\"}")
C=$(echo "$R" | json "['challengeId']"); O=$(echo "$R" | json "['devOtp']")
S=$(curl -sf --cacert "$CA" "$TLS/api/auth/verify" -H 'Content-Type: application/json' -d "{\"challengeId\":\"$C\",\"otp\":\"$O\"}")
T=$(echo "$S" | json "['accessToken']")
echo "login ok over TLS for $(echo "$S" | json "['user']['maskedPhone']") ($ROLE)"
for i in 1 2 3 4 5 6; do
  curl -sf "$BASE/api/me" -H "Authorization: Bearer $T" -o /dev/null -w '%{http_code} '
done
echo
echo "$T"
