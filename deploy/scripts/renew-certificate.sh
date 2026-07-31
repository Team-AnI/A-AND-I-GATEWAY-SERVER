#!/usr/bin/env bash

set -Eeuo pipefail

DOMAIN="api.aandiclub.com"
CERTBOT_BASE_DIR="/opt/aandi/gateway/certbot"
CERTBOT_CONF_DIR="${CERTBOT_BASE_DIR}/conf"
CERTBOT_WEBROOT_DIR="${CERTBOT_BASE_DIR}/www"
CERTIFICATE_PATH="${CERTBOT_CONF_DIR}/live/${DOMAIN}/fullchain.pem"
PRIVATE_KEY_PATH="${CERTBOT_CONF_DIR}/live/${DOMAIN}/privkey.pem"
NGINX_CONTAINER="aandi-gateway-nginx"
CERTBOT_IMAGE="certbot/certbot@sha256:34ee91d2f43008eb78a007d22f23ed4b2eaa9a454cb27ca2c042b49527a695b4"

sudo test -f "${CERTIFICATE_PATH}" || {
  echo "ERROR: certificate not found: ${CERTIFICATE_PATH}" >&2
  exit 1
}
sudo test -f "${PRIVATE_KEY_PATH}" || {
  echo "ERROR: private key not found: ${PRIVATE_KEY_PATH}" >&2
  exit 1
}

nginx_running="$(sudo docker inspect --format '{{.State.Running}}' "${NGINX_CONTAINER}")"
if [ "${nginx_running}" != "true" ]; then
  echo "ERROR: ${NGINX_CONTAINER} is not running" >&2
  exit 1
fi

sudo openssl version >/dev/null

sudo docker run --rm --pull=always \
  -v "${CERTBOT_WEBROOT_DIR}:/var/www/certbot" \
  -v "${CERTBOT_CONF_DIR}:/etc/letsencrypt" \
  "${CERTBOT_IMAGE}" renew \
  --cert-name "${DOMAIN}" \
  --webroot \
  -w /var/www/certbot \
  --non-interactive \
  --no-random-sleep-on-renew

sudo openssl x509 \
  -checkend 1209600 \
  -noout \
  -in "${CERTIFICATE_PATH}"

sudo docker exec "${NGINX_CONTAINER}" nginx -t
sudo docker exec "${NGINX_CONTAINER}" nginx -s reload

sudo openssl x509 \
  -noout \
  -subject \
  -issuer \
  -serial \
  -dates \
  -in "${CERTIFICATE_PATH}"
