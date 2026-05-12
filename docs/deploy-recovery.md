# Deploy Recovery Guide

## v2.0.14 배포 실패 확인

운영 EC2에서 현재 상태를 먼저 확인한다.

```bash
cd /opt/aandi/gateway

sudo docker ps -a --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
sudo docker compose config -q

sudo docker logs --tail=200 aandi-gateway-server 2>&1 || true
sudo docker logs --tail=200 aandi-gateway-discord-bot 2>&1 || true
sudo docker exec aandi-gateway-nginx nginx -t 2>&1 || true
sudo docker logs --tail=100 aandi-gateway-nginx 2>&1 || true
```

`healthcheck.test must start either by "CMD", "CMD-SHELL" or "NONE"`가 나오면 monitor-bot healthcheck 형식 문제다.

## v2.0.13 Gateway Rollback

Gateway만 v2.0.13으로 되돌린다. Redis volume과 monitor-bot state volume은 삭제하지 않는다.

```bash
cd /opt/aandi/gateway

ECR_REGISTRY="362622729632.dkr.ecr.ap-northeast-2.amazonaws.com"
GATEWAY_IMAGE="${ECR_REGISTRY}/aandi-gateway-server:v2.0.13"

sudo cp docker-compose.yml "docker-compose.yml.before-rollback-$(date +%Y%m%d%H%M%S)"
sudo sed -i -E "s#image: .*/aandi-gateway-server:v[0-9.]+#image: ${GATEWAY_IMAGE}#" docker-compose.yml

sudo docker compose config -q
sudo docker compose pull gateway
sudo docker compose up -d --no-deps gateway
```

Gateway health를 확인한다.

```bash
NETWORK="$(sudo docker inspect aandi-gateway-server --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}')"

sudo docker run --rm --network "$NETWORK" curlimages/curl:8.10.1 \
  -fsS --max-time 3 http://gateway:9090/actuator/health
```

nginx 설정을 검증한 뒤 reload한다.

```bash
sudo docker exec aandi-gateway-nginx nginx -t
sudo docker exec aandi-gateway-nginx nginx -s reload
```

monitor-bot이 계속 실패하거나 메모리 여유가 부족하면 monitor-bot만 내린다.

```bash
sudo docker rm -f aandi-gateway-discord-bot || true
```

## 추가 진단

Docker 이벤트와 리소스를 확인한다.

```bash
sudo journalctl -u docker --since "30 minutes ago" --no-pager | tail -n 200

free -h
vmstat 1 5
df -hT
sudo docker stats --no-stream --format 'table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}'
```
