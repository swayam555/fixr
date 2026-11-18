#!/bin/sh
# One-shot: creates the JWT signing key and a local CA + TLS cert for nginx, once.
set -eu
OUT=/out
mkdir -p "$OUT/keys" "$OUT/certs"
if [ ! -f "$OUT/keys/jwt-private.pem" ]; then
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$OUT/keys/jwt-private.pem"
  echo "created JWT key"
fi
if [ ! -f "$OUT/certs/server.pem" ]; then
  openssl req -x509 -newkey rsa:2048 -nodes -days 825 -subj "/CN=Fixr Local CA" \
    -keyout "$OUT/certs/ca.key" -out "$OUT/certs/ca.pem"
  cat > /tmp/san.ext <<SAN
subjectAltName=DNS:localhost,DNS:fixr.local,DNS:nginx,IP:127.0.0.1,IP:10.0.2.2
extendedKeyUsage=serverAuth
SAN
  openssl req -newkey rsa:2048 -nodes -subj "/CN=fixr.local" -keyout "$OUT/certs/server.key" -out /tmp/server.csr
  openssl x509 -req -in /tmp/server.csr -CA "$OUT/certs/ca.pem" -CAkey "$OUT/certs/ca.key" -CAcreateserial \
    -days 825 -extfile /tmp/san.ext -out "$OUT/certs/server.pem"
  echo "created local CA and TLS cert"
fi
chmod 644 "$OUT/keys/jwt-private.pem" "$OUT/certs/server.key"
