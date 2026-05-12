# Docker Cleanup Systemd Timer

## 목적

Amazon Linux 2023에서는 cron보다 systemd timer로 주기 작업을 관리한다. Docker image/cache 정리는 앱 내부 Quartz Cron이 아니라 EC2 host의 systemd timer로 처리한다.

주의:

- Redis volume 삭제 금지
- monitor-bot state volume 삭제 금지
- `docker system prune --volumes` 사용 금지
- `docker volume prune` 사용 금지
- `docker network prune` 사용 금지
- 실행 중인 container에는 영향을 주지 않음
- rollback용 이전 image가 사라질 수 있으므로 14일 이내 image/cache는 남김

## Disk 확인

설치 전후 또는 수동 실행 전후에 아래 명령으로 상태를 확인한다.

```bash
df -hT
sudo docker system df
sudo docker image ls
```

## Service

```bash
sudo tee /etc/systemd/system/aandi-docker-cleanup.service >/dev/null <<'EOF'
[Unit]
Description=A&I Docker cleanup without deleting volumes
Wants=docker.service
After=docker.service

[Service]
Type=oneshot
ExecStart=/usr/bin/docker image prune -af --filter "until=336h"
ExecStart=/usr/bin/docker builder prune -af --filter "until=336h"
ExecStart=/usr/bin/docker container prune -f --filter "until=336h"
EOF
```

## Timer

2주에 한 번 실행한다.

```bash
sudo tee /etc/systemd/system/aandi-docker-cleanup.timer >/dev/null <<'EOF'
[Unit]
Description=Run A&I Docker cleanup every 2 weeks

[Timer]
OnCalendar=Sun *-*-1,15 04:30:00
Persistent=true
RandomizedDelaySec=900

[Install]
WantedBy=timers.target
EOF
```

활성화:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now aandi-docker-cleanup.timer
sudo systemctl list-timers | grep aandi-docker-cleanup
```

수동 실행:

```bash
sudo systemctl start aandi-docker-cleanup.service
sudo journalctl -u aandi-docker-cleanup.service -n 100 --no-pager
```

이 timer는 volume을 삭제하지 않는다. 운영 중인 Redis appendonly volume과 `/var/lib/monitor-bot` state volume을 보존한다.
