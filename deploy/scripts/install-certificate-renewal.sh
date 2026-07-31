#!/usr/bin/env bash

set -Eeuo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_DIR="/opt/aandi/gateway/scripts"
SYSTEMD_DIR="/etc/systemd/system"
SERVICE_NAME="aandi-certificate-renewal.service"
TIMER_NAME="aandi-certificate-renewal.timer"

sudo install -d -m 0755 "${INSTALL_DIR}"
sudo install -m 0755 \
  "${DEPLOY_DIR}/scripts/renew-certificate.sh" \
  "${INSTALL_DIR}/renew-certificate.sh"

sudo systemd-analyze verify \
  "${DEPLOY_DIR}/systemd/${SERVICE_NAME}" \
  "${DEPLOY_DIR}/systemd/${TIMER_NAME}"

sudo install -m 0644 \
  "${DEPLOY_DIR}/systemd/${SERVICE_NAME}" \
  "${SYSTEMD_DIR}/${SERVICE_NAME}"
sudo install -m 0644 \
  "${DEPLOY_DIR}/systemd/${TIMER_NAME}" \
  "${SYSTEMD_DIR}/${TIMER_NAME}"

sudo systemctl daemon-reload
sudo systemctl enable --now "${TIMER_NAME}"
sudo systemctl is-enabled --quiet "${TIMER_NAME}"
sudo systemctl is-active --quiet "${TIMER_NAME}"
sudo systemctl list-timers "${TIMER_NAME}" --no-pager
