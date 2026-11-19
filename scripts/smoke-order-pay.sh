#!/usr/bin/env bash
# Phase 2 end to end through nginx: book -> (simulated dispatch accept) -> job lifecycle -> pay via the
# local Razorpay stub -> replay the webhook -> refund. Checks the order.events stream and the ledger.
set -euo pipefail
cd "$(dirname "$0")/.."
BASE=${BASE:-http://localhost:8080}
RZP=${RZP:-http://localhost:8097}
j() { python3 -c "import sys,json;d=json.load(sys.stdin);print($1)"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

login() {
  local r c o
  r=$(curl -sf "$BASE/api/auth/otp" -H 'Content-Type: application/json' -d "{\"phone\":\"+9197$(shuf -i 10000000-99999999 -n1)\",\"role\":\"$1\"}")
  c=$(echo "$r" | j "d['challengeId']"); o=$(echo "$r" | j "d['devOtp']")
  curl -sf "$BASE/api/auth/verify" -H 'Content-Type: application/json' -d "{\"challengeId\":\"$c\",\"otp\":\"$o\"}"
}
call() { # method path token [body] [extra header]
  curl -s -X "$1" "$BASE$2" -H "Authorization: Bearer $3" -H 'Content-Type: application/json' ${5:+-H "$5"} ${4:+-d "$4"}
}

CS=$(login CUSTOMER); CT=$(echo "$CS" | j "d['accessToken']")
PS=$(login PRO);      PT=$(echo "$PS" | j "d['accessToken']"); PRO=$(echo "$PS" | j "d['user']['id']")
OS=$(login OPS);      OT=$(echo "$OS" | j "d['accessToken']")
echo "logged in customer, pro $PRO, ops"

ADDR=$(call POST /api/addresses "$CT" '{"label":"Home","line1":"5th Block","society":"Prestige Acropolis","flat":"B-1204","lat":12.9352,"lng":77.6245}' | j "d['id']")
KEY=$(cat /proc/sys/kernel/random/uuid)
O1=$(call POST /api/orders "$CT" "{\"serviceId\":\"tap-leak\",\"addressId\":\"$ADDR\"}" "Idempotency-Key: $KEY")
O2=$(call POST /api/orders "$CT" "{\"serviceId\":\"tap-leak\",\"addressId\":\"$ADDR\"}" "Idempotency-Key: $KEY")
ORDER=$(echo "$O1" | j "d['id']")
[ "$ORDER" = "$(echo "$O2" | j "d['id']")" ] || fail "idempotent booking returned two orders"
echo "booked $ORDER ($(echo "$O1" | j "d['status']"), ₹$(echo "$O1" | j "d['pricePaise']//100"))"

EVT=$(cat /proc/sys/kernel/random/uuid)
MSG="{\"eventId\":\"$EVT\",\"type\":\"ACCEPTED\",\"orderId\":\"$ORDER\",\"proId\":\"$PRO\",\"etaSec\":420,\"atMs\":$(date +%s%3N)}"
printf '%s|%s\n%s|%s\n' "$ORDER" "$MSG" "$ORDER" "$MSG" | docker compose exec -T -e KAFKA_HEAP_OPTS=-Xmx64m kafka \
  /opt/kafka/bin/kafka-console-producer.sh --bootstrap-server localhost:9092 --topic dispatch.results \
  --property parse.key=true --property key.separator='|' >/dev/null
for _ in $(seq 40); do
  ST=$(call GET "/api/orders/$ORDER" "$CT" | j "d['status']"); [ "$ST" = ASSIGNED ] && break; sleep 0.5
done
[ "$ST" = ASSIGNED ] || fail "order stuck in $ST after duplicate ACCEPTED"
CODE=$(call GET "/api/orders/$ORDER" "$CT" | j "d['startCode']")
for a in en-route arrived; do call POST "/api/orders/$ORDER/$a" "$PT" '{}' >/dev/null; done
call POST "/api/orders/$ORDER/start" "$PT" "{\"code\":\"$CODE\"}" >/dev/null
DONE=$(call POST "/api/orders/$ORDER/complete" "$PT" '{}' | j "d['status']")
[ "$DONE" = COMPLETED ] || fail "complete returned $DONE"
echo "job lifecycle ok: ASSIGNED -> EN_ROUTE -> ARRIVED -> IN_PROGRESS -> COMPLETED"

for _ in $(seq 40); do
  P=$(call POST "/api/payments/orders/$ORDER" "$CT"); echo "$P" | grep -q gatewayOrderId && break; sleep 0.5
done
GO=$(echo "$P" | j "d['gatewayOrderId']") || fail "no payment intent: $P"
AMT=$(echo "$P" | j "d['amountPaise']")
CK=$(curl -sf "$RZP/_local/checkout" -d "{\"order_id\":\"$GO\",\"method\":\"upi\"}")
call POST /api/payments/confirm "$CT" "$CK" | j "d['status']" >/dev/null || fail "checkout signature rejected"
for _ in $(seq 40); do
  ST=$(call GET "/api/payments/orders/$ORDER" "$CT" | j "d['status']"); [ "$ST" = CAPTURED ] && break; sleep 0.5
done
[ "$ST" = CAPTURED ] || fail "payment $ST, not CAPTURED"
echo "paid ₹$((AMT / 100)) via $GO, captured by webhook"

ledger() { call GET "/api/payments/ledger?prefix=liability:pro_payable:$PRO" "$OT" | j "d['accounts'][0]['paise'] if d['accounts'] else 0"; }
FEE=$((AMT * 2000 / 10000))
[ "$(ledger)" = "-$((AMT - FEE))" ] || fail "pro payable $(ledger), want -$((AMT - FEE))"
EV=$(curl -sf "$RZP/_local/webhooks" | python3 -c "import sys,json;print([w['eventId'] for w in json.load(sys.stdin) if w['event']=='payment.captured'][-1])")
for _ in 1 2 3; do curl -sf -X POST "$RZP/_local/webhooks/$EV/replay" >/dev/null; done
[ "$(ledger)" = "-$((AMT - FEE))" ] || fail "replayed webhook moved the ledger"
echo "ledger: pro owed ₹$(((AMT - FEE) / 100)), fee ₹$((FEE / 100)); 3 replays of $EV left it unchanged"

RK=$(cat /proc/sys/kernel/random/uuid)
for _ in 1 2; do call POST "/api/payments/orders/$ORDER/refunds" "$OT" "{\"amountPaise\":$AMT,\"reason\":\"smoke\"}" "Idempotency-Key: $RK" >/dev/null; done
for _ in $(seq 40); do
  ST=$(call GET "/api/payments/orders/$ORDER" "$CT" | j "d['status']"); [ "$ST" = REFUNDED ] && break; sleep 0.5
done
[ "$ST" = REFUNDED ] || fail "payment $ST after refund"
[ "$(ledger)" = 0 ] || fail "pro payable $(ledger) after full refund"
TOTAL=$(call GET /api/payments/ledger "$OT" | j "d['totalPaise']")
[ "$TOTAL" = 0 ] || fail "trial balance off by $TOTAL"
echo "refund (sent twice, same key) reversed the ledger; trial balance $TOTAL"

TYPES=$(docker compose exec -T -e KAFKA_HEAP_OPTS=-Xmx64m kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic order.events --from-beginning --timeout-ms 6000 2>/dev/null \
  | python3 -c "import sys,json
o='$ORDER'
for l in sys.stdin:
  try: e=json.loads(l)
  except ValueError: continue
  if e.get('orderId')==o: print(e['type'])")
echo "order.events: $(echo $TYPES)"
[ "$(echo "$TYPES" | grep -c '^ORDER_CREATED$')" = 1 ] || fail "expected exactly one ORDER_CREATED"
echo "SMOKE OK"
