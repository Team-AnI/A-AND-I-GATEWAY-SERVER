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
POST_SERVICE_URI=http://post-service.internal:8080

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

## 5) api.aandiclub.com HTTPS 설정

- DNS:
  - `api.aandiclub.com` A 레코드를 운영 EC2 Public IP(권장: EIP)로 연결
- GitHub Actions Secret:
  - `LETSENCRYPT_EMAIL`: Let's Encrypt 알림용 운영 이메일
- 보안그룹:
  - 인바운드 `80/tcp`, `443/tcp` 허용
  - `8080`, `6379`는 외부 미허용 유지
- 인증서 경로:
  - `deploy/certbot/conf/live/api.aandiclub.com/fullchain.pem`
  - `deploy/certbot/conf/live/api.aandiclub.com/privkey.pem`
- 인증서 발급(EC2에서 프로젝트 루트 기준):

```bash
mkdir -p deploy/certbot/www deploy/certbot/conf

docker run --rm \
  -v "$(pwd)/deploy/certbot/www:/var/www/certbot" \
  -v "$(pwd)/deploy/certbot/conf:/etc/letsencrypt" \
  certbot/certbot certonly --webroot \
  -w /var/www/certbot \
  -d api.aandiclub.com \
  --email <운영자-이메일> \
  --agree-tos \
  --no-eff-email
```

- 인증서 발급 후 서비스 시작/재시작:

```bash
docker compose up -d
docker compose restart nginx
```

## 6) 신규 서비스 도메인 미연결 상태에서의 권장안

- 우선 서비스 연결은 내부 DNS/프라이빗 IP로 진행:
  - 예: `NEW_SERVICE_URI=http://new-service.internal:8080`
- 외부 도메인은 준비되면 서브도메인으로 분리:
  - 예: `api.aandiclub.com`(gateway), `new-api.aandiclub.com`(신규 서비스)
- 단일 인스턴스에서 운영 시:
  - Nginx에 `server_name`/`location` 단위로 라우팅 추가
  - 서비스별 인증서(또는 `*.aandiclub.com` 와일드카드) 적용
- 도메인 연결 전 임시 검증:
  - 내부망/점프호스트에서만 접근 가능한 경로로 smoke test
  - `/etc/hosts` 임시 매핑으로 사전 점검 후 DNS 전환
