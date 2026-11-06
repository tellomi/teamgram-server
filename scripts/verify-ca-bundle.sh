#!/usr/bin/env bash
# Fail unless the runtime image has the CA bundle Go uses for TLS.
#
# crypto/x509 on Linux reads /etc/ssl/certs/ca-certificates.crt when
# SSL_CERT_FILE is unset, then certificates under /etc/ssl/certs when
# SSL_CERT_DIR is unset. Those two variables stay unset in this image.
# Pointing them at another path, or turning TLS verification off, is not
# a fix. Pass a fixture root as $1; the image build uses /.
set -euo pipefail

root="${1:-/}"
bundle="${root%/}/etc/ssl/certs/ca-certificates.crt"
certdir="${root%/}/etc/ssl/certs"

fail() {
  echo "verify-ca-bundle: $*" >&2
  exit 1
}

if [[ ! -s "$bundle" ]]; then
  fail "missing or empty ${bundle}. Install ca-certificates in the runtime image."
fi
if ! grep -q "BEGIN CERTIFICATE" "$bundle"; then
  fail "${bundle} has no PEM certificates."
fi
# The bundle file itself is one entry. An empty directory is the broken image.
if [[ ! -d "$certdir" ]] || [[ -z "$(find "$certdir" -mindepth 1 -print -quit)" ]]; then
  fail "${certdir} has no entries."
fi

# A set SSL_CERT_FILE or SSL_CERT_DIR replaces the defaults for every Go
# process. Only the real root is checked, so a fixture is not affected by
# the host environment.
if [[ "$root" == "/" ]]; then
  if [[ -n "${SSL_CERT_FILE:-}" ]]; then
    if [[ ! -s "$SSL_CERT_FILE" ]] || ! grep -q "BEGIN CERTIFICATE" "$SSL_CERT_FILE"; then
      fail "SSL_CERT_FILE=${SSL_CERT_FILE} is not a PEM bundle. Leave it unset."
    fi
  fi
  if [[ -n "${SSL_CERT_DIR:-}" ]]; then
    found=0
    if [[ -d "$SSL_CERT_DIR" ]]; then
      while IFS= read -r pem; do
        if grep -q "BEGIN CERTIFICATE" "$pem"; then
          found=1
          break
        fi
      done < <(find "$SSL_CERT_DIR" -type f -print)
    fi
    if [[ "$found" -ne 1 ]]; then
      fail "SSL_CERT_DIR=${SSL_CERT_DIR} has no certificates. Leave it unset."
    fi
  fi
fi
