# TLS Certificate Recovery Plan

## Goal

Restore trusted HTTPS for `api.aandiclub.com` and prevent certificate renewal from depending on application releases.

The v2 request headers remain unchanged. Public endpoints do not require JWT authentication, but clients still send `deviceOS` and an ISO-8601 `timestamp`.

## Confirmed root cause

- The served Let's Encrypt certificate expired on `2026-07-30 11:25:33 UTC`.
- Certificate renewal ran only at the end of a tag or manual CD deployment.
- The CD workflow ignored renewal failures and reloaded Nginx before renewal.
- No deployment ran during the certificate renewal window.

## Recovery

1. Run the `TLS Certificate Renewal` workflow manually from the default branch.
2. The workflow installs or refreshes the EC2 systemd timer and starts its oneshot service.
3. The service renews only the `api.aandiclub.com` certificate on EC2.
4. It verifies that the renewed certificate is valid for at least 14 days.
5. It validates the Nginx configuration and performs a graceful reload.
6. A GitHub-hosted runner verifies the certificate served publicly without bypassing TLS validation.
7. Verify `GET /v2/blogs?page=0&size=20` with the required `deviceOS` and ISO-8601 `timestamp` headers.

## Recovery result

The manual recovery workflow completed successfully on `2026-07-31 UTC`.

- Nginx loaded a new Let's Encrypt certificate after a successful configuration test.
- The served certificate expires on `2026-10-29 03:29:08 UTC`.
- Public TLS verification returns result code `0`.
- `GET /v2/blogs` returns `200` with the required v2 headers.

## Prevention

- Use the EC2 systemd timer as the primary scheduler at `01:17` and `13:17` UTC every day.
- Keep the GitHub workflow as a daily installer, backup trigger, and external verifier at `06:41` UTC.
- Stop a stuck renewal attempt after 8 minutes so later timer runs are not blocked.
- Serialize GitHub-triggered certificate maintenance and CD with the `production-ec2-maintenance` concurrency group.
- Keep certificate renewal independent from gateway and monitor-bot image deployment.
- Fail the workflow when renewal, certificate validity, Nginx validation, reload, or public TLS verification fails.
- Retain manual dispatch for recovery and operational verification.
- Do not rely only on a scheduled GitHub workflow because public repositories disable scheduled workflows after 60 days without repository activity.

## Residual operational risk

The host timer records failures in the systemd journal. The daily GitHub workflow provides a second execution path and an external TLS check while scheduled workflows remain active.

An alert channel that is independent from GitHub Actions is not provisioned by this repository. Configure an external certificate-expiry alert with a 14-day threshold so repeated host failures remain visible even if scheduled GitHub workflows are disabled.

## Rollback

The change does not replace or delete existing certificate files manually. Certbot retains the lineage under `/opt/aandi/gateway/certbot/conf`.

If Nginx validation fails, the script exits before reload and the running Nginx process remains unchanged. If a reload must be reversed, restore the previous Certbot lineage on EC2, run `nginx -t`, and reload Nginx again.

Disable the host schedule without deleting certificates:

```bash
sudo systemctl disable --now aandi-certificate-renewal.timer
```
