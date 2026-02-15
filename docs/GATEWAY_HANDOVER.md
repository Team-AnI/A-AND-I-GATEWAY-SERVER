# Gateway 인수인계 문서 (단일본)

기준일: 2026-02-15  
대상 레포: [Team-AnI/A-AND-I-GATEWAY-SERVER](https://github.com/Team-AnI/A-AND-I-GATEWAY-SERVER)  
기준 브랜치: `main`

## 1) 목적과 역할

- 이 Gateway는 `Access Token 검증 전용` 서버로 사용한다.
- 검증 성공 요청만 하위 MSA 서비스로 라우팅한다.
- 게이트웨이는 공통 기능(토큰 검증, 라우팅, 공통 헤더 전달)만 담당한다.

### 비범위

- 로그인/회원가입/토큰 발급
- 도메인별 상세 인가 정책
- 비즈니스 로직 처리

## 2) 현재 구현 상태

### 보안/인증

- OAuth2 Resource Server(JWT) 기반 토큰 검증 적용
- JWT `issuer` + `audience` 검증 적용
- 내부 무효화 웹훅은 `X-Internal-Token`으로 별도 보호
- 공개 엔드포인트:
  - `/actuator/health`
  - `/actuator/health/**`
  - `/actuator/info`
- 그 외 엔드포인트: 인증 필수

### 사용자 정보 전달

- 인증된 요청에 대해 하위 서비스로 헤더 전달
  - `X-User-Id`
  - `X-Roles`

### 라우팅 매핑

- `/v2/report/**` -> `REPORT_SERVICE_URI` (기본 `http://localhost:8081`)
- `/v2/user/**` -> `USER_SERVICE_URI` (기본 `http://localhost:8082`)
- `/v2/admin/**` -> `ADMIN_SERVICE_URI` (기본 `http://localhost:8083`)
- 기본 Path 변환: 라우트별 `StripPrefix=2`

### 적용 파일

- `build.gradle.kts`
- `src/main/resources/application.yaml`
- `src/main/kotlin/com/aandi/gateway/security/SecurityConfig.kt`
- `src/main/kotlin/com/aandi/gateway/security/AuthenticatedPrincipalHeaderFilter.kt`
- `src/test/kotlin/com/aandi/gateway/security/SecurityConfigTests.kt`
- `src/test/kotlin/com/aandi/gateway/security/AuthenticatedPrincipalHeaderFilterTests.kt`

## 3) 실행/검증 방법

### 필수 환경변수

- `AUTH_ISSUER_URI`
- `AUTH_JWK_SET_URI`
- `AUTH_AUDIENCE`
- `INTERNAL_EVENT_TOKEN`
- `REDIS_HOST`
- `REDIS_PORT`
- `REDIS_PASSWORD`
- `REPORT_SERVICE_URI`
- `USER_SERVICE_URI`
- `ADMIN_SERVICE_URI`

### 로컬 검증

```bash
./gradlew test
curl http://localhost:9090/actuator/health
```

`docker-compose` 실행 기준으로는 `9090`이 외부로 publish되지 않으므로 아래처럼 컨테이너 내부에서 확인한다.

```bash
docker compose exec gateway sh -c "wget -qO- http://127.0.0.1:9090/actuator/health"
```

## 4) Redis 캐시 전략 (API/DB 호출 최소화)

목표: 동일 사용자/토큰 관련 반복 요청 시 외부 API/DB 호출을 줄이고 응답 지연을 낮춘다.

### 정책

- 첫 요청 시:
  - 서버가 필요한 JSON 데이터를 생성/조회
  - Redis에 저장
  - 저장 시점 기준 타임스탬프/TTL 24시간 설정
- 24시간 이내 재요청 시:
  - Redis JSON 사용
  - DB/외부 API 재호출 없이 처리
- 24시간 경과 후 요청 시:
  - 원본 소스(DB/외부 API) 재요청
  - Redis 갱신 후 응답

### 권장 키 설계

- 데이터 키: `cache:token:{subject}:{tokenHash}`
- 인덱스 키: `cache:token-index:{subject}`
- 값: JSON payload
- 만료: `EXPIRE 86400` (24시간)

토큰 원문 저장은 피하고 `token hash`를 사용한다.

### 처리 플로우

1. 요청 수신 후 토큰 검증
2. 캐시 키 생성
3. Redis 조회
4. 캐시 hit면 바로 응답 처리
5. 캐시 miss 또는 만료면 원본 조회 후 Redis 저장 + subject 인덱스 등록
6. 하위 서비스 라우팅/응답

### 강제 무효화 트리거 (확정)

- 로그아웃 이벤트 수신 시: subject 인덱스 기준 캐시 일괄 삭제
- 권한 변경 이벤트 수신 시: subject 인덱스 기준 캐시 일괄 삭제
- 구현 진입점: `TokenContextInvalidationService`

### 운영 시 주의사항

- 사용자 권한 변경 즉시 반영이 필요하면 24시간 TTL은 길 수 있음
- 강제 무효화는 subject 인덱스 기반으로 처리
- 캐시 데이터 스키마 버전 변경 시 키 prefix 버저닝 권장

### 운영 보안 고정 (확정)

- Actuator는 별도 포트/주소로 분리
  - `MANAGEMENT_SERVER_PORT` 기본 `9090`
  - `MANAGEMENT_SERVER_ADDRESS` 기본 `127.0.0.1`
- 컨테이너 구성 (같은 EC2, Docker Compose 기준)
  - Nginx: 외부 공개 포트 `80` (EC2 보안그룹에서 `0.0.0.0/0` 허용)
  - Gateway `8080`: `expose`만 설정, Docker 내부 네트워크 전용 — 호스트에 미노출
  - Redis `6379`: `expose`만 설정, Docker 내부 네트워크 전용 — 호스트에 미노출
  - Actuator `9090`: `127.0.0.1` 바인딩, 외부/호스트 모두 차단
- EC2 보안그룹 규칙
  - `80` (HTTP): 외부 허용 (Nginx 진입점)
  - `443` (HTTPS): 외부 허용 (SSL 적용 시)
  - `8080`, `6379`, `9090`: 외부 차단 — 인바운드 규칙 추가하지 않음
- Nginx `/internal/` 접근 제한
  - VPC CIDR `172.31.0.0/16` 내부만 허용
  - 외부 인터넷 deny
- Nginx `/actuator/` 완전 차단 (return 403)

## 5) 미결정 항목 (운영 확정 필요)

- 무효화 이벤트 채널을 웹훅에서 메시지 브로커로 전환할지 여부
- 하위 서비스가 신뢰할 내부 헤더 표준 확정

## 6) 컨테이너 레지스트리 연동 (GHCR 기준)

현재 배포 방식은 Docker Hub가 아니라 `GHCR(ghcr.io)`를 사용한다.

### 핵심 정리

- Docker Hub 계정 연동은 필요 없다.
- GitHub Actions가 `ghcr.io`로 이미지를 푸시한다.
- EC2는 `ghcr.io`에서 이미지를 pull 받아 컨테이너를 실행한다.

### CI/CD 동작 방식

- 워크플로우 파일: `.github/workflows/cd.yml`
- 트리거: `vX.Y.Z` 형식 태그 push
- 수행:
  - `./gradlew build`
  - `ghcr.io` 로그인
  - 이미지 push
- 태그:
  - `ghcr.io/<owner>/<repo>:latest`
  - `ghcr.io/<owner>/<repo>:<git-tag>`

### GitHub 권한/설정

- Repository Actions 권한:
  - `packages: write` 필요 (workflow에 이미 설정됨)
- 이미지 pull 대상(EC2/런타임)의 권한:
  - private 패키지면 PAT 또는 GitHub App 토큰으로 `read:packages` 필요
  - public 패키지면 익명 pull 가능(조직 정책에 따라 제한 가능)

### EC2 배포 시 기본 절차

```bash
# private GHCR인 경우
echo $GHCR_TOKEN | docker login ghcr.io -u <github-username> --password-stdin

docker pull ghcr.io/team-ani/a-and-i-gateway-server:latest
```

운영 배포는 반드시 `docker-compose`를 사용한다.
`gateway` 컨테이너에 `-p 8080:8080` 옵션을 직접 주면 Nginx를 우회해 외부에서 직접 접근 가능해지므로 절대 사용하지 않는다.

```bash
# 운영 환경변수 파일 준비 후
docker-compose -f docker-compose.yml up -d
```

## 7) 인수인계 체크리스트

- [ ] 운영 환경변수 주입 완료
- [ ] issuer/jwk 통신 확인 완료
- [ ] 공개/인증필수 엔드포인트 정책 검증 완료
- [ ] Redis TTL 24시간 정책 합의 완료
- [ ] 캐시 무효화 시나리오 정의 완료
- [ ] 장애 대응 담당자/채널 확정
