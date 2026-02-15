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
- 운영에서 경로 규칙이 다르면 `REPORT_STRIP_PREFIX`, `USER_STRIP_PREFIX`, `ADMIN_STRIP_PREFIX`로 조정

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
- `REDIS_HOST`
- `REDIS_PORT`
- `REDIS_PASSWORD`
- `REPORT_SERVICE_URI`
- `USER_SERVICE_URI`
- `ADMIN_SERVICE_URI`
- `REPORT_STRIP_PREFIX`
- `USER_STRIP_PREFIX`
- `ADMIN_STRIP_PREFIX`

### 로컬 검증

```bash
./gradlew test
curl http://localhost:8080/actuator/health
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

- 키 패턴: `cache:token:{subject-or-token-hash}`
- 값: JSON payload
- 만료: `EXPIRE 86400` (24시간)

토큰 원문 저장은 피하고, `token hash` 또는 `subject + scope` 조합을 권장한다.

### 처리 플로우

1. 요청 수신 후 토큰 검증
2. 캐시 키 생성
3. Redis 조회
4. 캐시 hit면 바로 응답 처리
5. 캐시 miss 또는 만료면 원본 조회 후 Redis 저장
6. 하위 서비스 라우팅/응답

### 운영 시 주의사항

- 사용자 권한 변경 즉시 반영이 필요하면 24시간 TTL은 길 수 있음
- 강제 무효화가 필요하면 별도 invalidate 키/이벤트 전략 필요
- 캐시 데이터 스키마 버전 변경 시 키 prefix 버저닝 권장

## 5) 미결정 항목 (운영 확정 필요)

- `audience` 검증 적용 여부
- Redis 캐시 키 기준: `subject`, `token hash`, `subject+scope` 중 최종 선택
- 권한 변경/강제 로그아웃 시 캐시 무효화 방식
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
docker stop aandi-gateway || true
docker rm aandi-gateway || true
docker run -d --name aandi-gateway -p 8080:8080 \
  -e AUTH_ISSUER_URI=... \
  -e AUTH_JWK_SET_URI=... \
  -e REDIS_HOST=... \
  -e REDIS_PORT=6379 \
  -e REDIS_PASSWORD=... \
  ghcr.io/team-ani/a-and-i-gateway-server:latest
```

## 7) 인수인계 체크리스트

- [ ] 운영 환경변수 주입 완료
- [ ] issuer/jwk 통신 확인 완료
- [ ] 공개/인증필수 엔드포인트 정책 검증 완료
- [ ] Redis TTL 24시간 정책 합의 완료
- [ ] 캐시 무효화 시나리오 정의 완료
- [ ] 장애 대응 담당자/채널 확정
