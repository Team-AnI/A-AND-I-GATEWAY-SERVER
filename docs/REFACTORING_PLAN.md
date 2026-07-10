# 안정적 리팩터링 실행 계획

기준일: 2026-07-10

## 원칙

- 한 단계에서는 한 종류의 위험만 다룬다.
- 구조를 옮기기 전에 현재 동작을 회귀 테스트로 고정한다.
- 공개 API의 path, method, status, error code는 별도 합의 없이 변경하지 않는다.
- 각 단계는 독립적으로 배포하고 되돌릴 수 있는 크기로 나눈다.
- 관련 테스트와 빌드가 모두 통과하지 않으면 다음 단계로 넘어가지 않는다.

## 단계별 상태

| 단계 | 목표 | 상태 | 완료 조건 |
|---|---|---|---|
| 0 | 기준선 확보 | 완료 | Gateway, Monitor Bot, 성능 자산 검증 통과 |
| 1 | 장애·메모리 위험 차단 | 구현 완료, 커밋 전 | 전체 테스트와 빌드 통과, 배포 설정 정적 검증 통과 |
| 2 | 실행·연동 문서 정확성 복구 | 완료(Docker 항목 제외) | 환경 변수 전수 대조, 문서 계약 테스트, 전체 회귀 검증 통과 |
| 3 | 배포 설정 source of truth 단일화 | 보류(Docker 범위) | production Compose/nginx를 workflow 밖의 추적 파일로 이동 |
| 4 | route와 보안 정책 source of truth 단일화 | 4A 완료, 4B 다음 | 실제 route 선택과 route별 선언 method 도달성 검사 후 typed contract로 단계적 전환 |
| 5 | Monitor Bot 대형 모듈 분리 | 계획 | 동작 변경 없는 파일 분리 후 Go 전체 테스트와 race 통과 |
| 6 | 레거시 설정·문서 정리 | 계획 | 사용처 증명 후 제거, 문서 drift 검사를 CI에 연결 |

## 1단계: 장애·메모리 위험 차단

이번 작업에서 다음을 처리한다.

- Redis가 배포 시 함께 시작되고 health check가 비밀번호 인증을 사용하도록 수정하며 AOF를 persistent volume에 보존한다.
- Gateway 배포 판정을 전체 Actuator health로 변경해 Redis 장애를 감지한다.
- Monitor Bot은 HTTP, AWS SDK, Discord application ID, 유효한 Discord 서명 키, interaction handler가 모두 준비된 뒤에만 `/healthz`에서 200을 반환한다.
- CloudWatch 일부 조회 실패나 query budget 초과가 기존 alert의 거짓 resolved 알림으로 이어지지 않게 한다.
- 요청·응답 원문은 streaming으로 전달하고, 로그에는 64 KiB 이하의 body만 기록하며 초과 body는 marker로 대체한다.
- auth request body는 공통 bounded cache에서 한 번만 읽고 최대 크기 초과 시 표준 413 응답을 반환한다.
- rate-limit은 분 단위로 초기화되는 고정 슬롯의 4-row 보수적 Count-Min counter를 사용해 입력 cardinality와 무관하게 메모리 사용을 고정한다. Hash 충돌은 제한을 우회시키지 않지만 드물게 false-positive 429를 만들 수 있다.

검증 gate:

- `./gradlew test bootJar --rerun-tasks --no-daemon`
- `cd monitor-bot && go test -race ./... && go vet ./...`
- 비밀번호 유무 각각 `docker compose config -q`
- `.github/workflows/cd.yml` YAML parse
- `git diff --check`

## 2단계: 문서 정확성 복구

Docker/Compose/CD와 이에 의존하는 README 로컬 실행 절차는 사용자 요청에 따라 이번 단계에서 제외했다. 문서 변경을 중심으로 진행하고, 문서 검증 중 발견한 Monitor Bot readiness 계약 불일치만 함께 수정했다.

1. `.env.example`에 Gateway와 Monitor Bot의 77개 환경 변수를 모두 반영하고 required, optional, secret, default를 표시한다.
2. `docs/SERVICE_GATEWAY_INTEGRATION.md`를 현재 Auth, Report, Post, Online Judge route와 역할 정책 기준으로 다시 작성한다.
3. Report OpenAPI route의 `order=-1` 우선순위와 rewrite를 characterization test로 고정한다.
4. `monitor-bot/README.md`의 PR 시점 문구와 조건부 설정을 정리한다.
5. `DISCORD_APPLICATION_ID`가 없을 때 `/healthz`가 준비 완료를 반환하지 않도록 문서와 readiness를 일치시킨다.
6. error enum과 `docs/GATEWAY_ERROR_CODES.md`의 code, HTTP, value, message, alert 완전성 테스트를 추가한다.

검증 gate:

- `.env.example`과 Gateway/Monitor Bot 설정 key 집합 일치(77/77)
- `zsh -n .env.example`
- `./gradlew test bootJar --rerun-tasks --no-daemon`
- `cd monitor-bot && go test -race ./... && go vet ./...`
- 로컬 Markdown 링크 검사
- `git diff --check`

## 3단계: 배포 설정 단일화

현재 사용자 요청에서는 Docker 범위이므로 보류한다.

현재 `.github/workflows/cd.yml`의 긴 heredoc이 production Compose와 nginx 설정을 생성한다. 먼저 생성 결과를 fixture로 고정한 뒤 아래 순서로 이동한다.

1. production Compose 템플릿을 `deploy/` 아래 추적 파일로 추출한다.
2. nginx 설정을 동일하게 추출한다.
3. workflow는 환경 값 주입과 배포 명령만 담당하게 줄인다.
4. `docker compose config`와 `nginx -t`를 CI gate로 추가한다.

이 단계에서는 container 이름, network, volume, health check, 시작 순서를 변경하지 않는다.

## 4단계: route와 보안 정책 단일화

`application.yaml`의 141개 route와 `GatewayRequestPolicyFilter.kt`의 method/path 정책이 별도로 관리된다. 한 번에 합치지 않고 4A 계약 고정과 4B 구조 전환으로 나눈다.

### 4A: 현재 계약 고정 — 완료

1. route definition과 실제 정렬된 `RouteLocator`가 모두 141개이며 ID가 고유한지 검사한다.
2. Auth 56, Report 21, Post 44, Online Judge 20 route 구성을 snapshot으로 고정한다.
3. Method가 명시된 118개 route는 선언된 모든 method가 request policy를 통과하고 해당 route로 최초 선택되는지 검사한다.
4. Method가 없는 23개 route는 GET/POST/PUT/PATCH/DELETE 중 하나 이상의 유효한 조합으로 도달 가능한지 검사한다.
5. OpenAPI 8개 route를 GET 전용으로 만들고 root `order=-2`, subpath `order=-1`로 우선순위를 명시한다.
6. `/v2/post/(admin/)courses...` Report alias를 Post catch-all보다 높은 order로 고정한다.
7. Report OpenAPI의 비-GET 요청과 `/v2/post/courses`의 PATCH/DELETE 교차 허용을 404로 차단한다.

4A 검사는 route에서 policy로 향하는 도달성과 실제 최초 선택을 고정한다. allow rule 전체의 역방향 orphan 검사와 broad `/**` 내부 모든 세부 method/path의 완전성은 4B typed contract 전환에서 다룬다.

검증 gate:

- `./gradlew test --tests com.aandi.gateway.security.GatewayRoutePolicyContractTests --no-daemon`
- `./gradlew test bootJar --rerun-tasks --no-daemon`
- `git diff --check`

### 4B: typed contract 전환 — 다음

1. allow/deny rule을 `GatewayEndpointContract`로 추출하고 ROUTED, LOCAL endpoint를 구분한다.
2. Auth부터 서비스 하나씩 method, path, role, upstream, order, rewrite 정보를 contract로 옮긴다.
3. 각 서비스 전환마다 기존 YAML route와 생성 결과의 동등성 테스트를 먼저 통과시킨다.
4. 모든 서비스 전환 후 `GatewayRequestPolicyFilter`와 `SecurityConfig`가 동일 contract를 소비하도록 변경한다.
5. 실제 handler나 route가 없는 legacy allow rule은 별도 테스트와 사용처 확인 후 제거한다.

경로 rewrite, 역할 인가, public/protected 구분은 각각 별도 변경으로 진행한다.

## 5단계: Monitor Bot 모듈 분리

현재 큰 파일은 `assignment_ops.go` 1,568줄, `discord_format.go` 1,560줄, `interactions.go` 1,542줄이다. 먼저 함수 이동만 수행하고 로직 개선은 후속 단계로 미룬다.

1. interaction command family별 handler 분리
2. assignment 수집, 진단, lifecycle, formatting 분리
3. Discord formatting을 dashboard, alert, assignment 단위로 분리

각 이동은 한 파일군씩 수행하고 `go test -race ./...`를 통과해야 한다.

## 6단계: 레거시 정리

- `.env.example`, Compose, 문서의 환경 변수는 코드 사용처와 대조한 뒤 제거한다.
- 완료된 측정 계획과 PR 시점 기록은 `docs/archive/`로 이동한다.
- error, env, route, metrics 문서의 drift 검사를 CI에 연결한다.
- Kotlin/Jackson deprecation은 동작 테스트를 유지한 별도 PR에서 제거한다.

삭제는 `rg` 사용처 확인과 clean clone 검증을 모두 통과한 항목에만 적용한다.
