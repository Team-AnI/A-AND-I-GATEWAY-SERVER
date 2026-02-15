# EC2 Environment Setup

기준일: 2026-02-15

## 1) 파일 위치

- 운영 EC2에 `/opt/aandi/gateway/.env` 파일을 생성한다.
- `docker compose --env-file /opt/aandi/gateway/.env up -d` 로 실행한다.

## 2) 필수 환경변수

```env
AUTH_ISSUER_URI=https://auth.example.com
AUTH_JWK_SET_URI=https://auth.example.com/.well-known/jwks.json
AUTH_AUDIENCE=aandi-gateway
INTERNAL_EVENT_TOKEN=replace-with-strong-random-token

REPORT_SERVICE_URI=http://report-service.internal:8080
USER_SERVICE_URI=http://user-service.internal:8080
ADMIN_SERVICE_URI=http://admin-service.internal:8080

REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=

TOKEN_CACHE_TTL=24h

MANAGEMENT_SERVER_PORT=9090
MANAGEMENT_SERVER_ADDRESS=127.0.0.1
```

## 3) 값 설정 가이드

- `AUTH_ISSUER_URI`: 인증 서버 issuer URL
- `AUTH_JWK_SET_URI`: JWT 서명키 조회 URL
- `AUTH_AUDIENCE`: Gateway 수신 대상 audience
- `INTERNAL_EVENT_TOKEN`: 내부 무효화 웹훅 보호 토큰 (랜덤 강력값)
- `*_SERVICE_URI`: 각 서비스의 내부 접근 주소
- `REDIS_*`: Redis 연결 정보
- `TOKEN_CACHE_TTL`: 토큰 컨텍스트 캐시 TTL

## 4) 운영 체크

- `.env`는 git에 커밋하지 않는다.
- `INTERNAL_EVENT_TOKEN`은 Secrets Manager/SSM에서 주입 권장
- 배포 후 내부에서 health 확인:
  - `docker compose exec gateway sh -c "wget -qO- http://127.0.0.1:9090/actuator/health"`
