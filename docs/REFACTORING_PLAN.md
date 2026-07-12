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
| 1 | 장애·메모리 위험 차단 | 완료 | 전체 테스트와 빌드 통과, 배포 설정 정적 검증 통과 |
| 2 | 실행·연동 문서 정확성 복구 | 완료 | 환경 변수 전수 대조, 문서 계약 테스트, 전체 회귀 검증 통과 |
| 3 | 배포 설정 source of truth 단일화 | 완료 | production Compose/nginx를 workflow 밖의 추적 파일로 이동 |
| 4 | route와 보안 정책 source of truth 단일화 | 4A, 4B-1, 4B-2a, 4B-2b, 4B-2c, 4B-2d 완료; 후속 단계 계획 필요 | 실제 route 선택과 route별 선언 method 도달성 검사 후 typed contract로 단계적 전환 |
| 5 | Monitor Bot 대형 모듈 분리 | 완료 | 동작 변경 없는 파일 분리 후 Go 전체 테스트와 race 통과 |
| 6 | 레거시 설정·문서 정리 | 완료 | 사용처 증명 후 제거, 문서 drift 검사를 CI에 연결 |

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

## 3단계: 배포 설정 단일화 — 완료

1. production 및 인증서 bootstrap Compose를 `deploy/` 아래 추적 파일로 추출했다.
2. nginx production 설정과 bootstrap template을 추적 파일로 분리했다.
3. workflow는 설정 검증, 파일 전송, domain 주입과 배포 명령만 담당하도록 줄였다.
4. `docker compose config`와 nginx 설정 검사를 CD validation gate로 추가하고 EC2의 기존 `nginx -t` gate를 유지했다.

이 단계에서는 container 이름, network, volume, health check, 시작 순서를 변경하지 않는다.

## 4단계: route와 보안 정책 단일화

`application.yaml`의 141개 route와 `GatewayRequestPolicyFilter.kt`의 method/path 정책이 별도로 관리된다. 한 번에 합치지 않고 4A 계약 고정, 4B-1 테스트 전용 route catalog dry-run, 4B-2 ordered endpoint policy/access catalog 순서로 전환한다.

### 4A: 현재 계약 고정 — 완료

1. route definition과 실제 정렬된 `RouteLocator`가 모두 141개이며 ID가 고유한지 검사한다.
2. Auth 56, Report 21, Post 44, Online Judge 20 route 구성을 snapshot으로 고정한다.
3. Method가 명시된 118개 route는 선언된 모든 method가 request policy를 통과하고 해당 route로 최초 선택되는지 검사한다.
4. Method가 없는 23개 route는 GET/POST/PUT/PATCH/DELETE 중 하나 이상의 유효한 조합으로 도달 가능한지 검사한다.
5. OpenAPI 8개 route를 GET 전용으로 만들고 root `order=-2`, subpath `order=-1`로 우선순위를 명시한다.
6. `/v2/post/(admin/)courses...` Report alias를 Post catch-all보다 높은 order로 고정한다.
7. Report OpenAPI의 비-GET 요청과 `/v2/post/courses`의 PATCH/DELETE 교차 허용을 404로 차단한다.
8. Host 정책의 private/loopback 허용은 요청 처리 중 DNS를 조회하지 않고 IP literal만 판정해, hostname의 DNS 결과가 내부 주소라는 이유로 허용되지 않도록 한다.
9. 사용자 정의 `WebFilter`의 실행 순서와 Spring Security 경계(CORS, 로깅, 요청 정책, backend availability, auth rate limit, auth request validation, Security, token/principal header)를 contract test로 고정한다.

4A 검사는 route에서 policy로 향하는 도달성과 실제 최초 선택을 고정한다. allow rule 전체의 역방향 orphan 검사와 broad `/**` 내부 모든 세부 method/path의 완전성은 4B-2 ordered endpoint policy/access catalog에서 다룬다.

검증 gate:

- `./gradlew test --tests com.aandi.gateway.security.GatewayRoutePolicyContractTests --no-daemon`
- `./gradlew test bootJar --rerun-tasks --no-daemon`
- `git diff --check`

### 4B: typed contract 전환 — 진행 중

#### 4B-1: Auth route catalog test-only dry-run — 완료

1. runtime source를 변경하지 않고 test source set에 `GatewayRouteContract`와 Auth catalog를 추가해 구조를 먼저 검증한다.
2. Auth 56개 route의 ID, 선언 순서, target, path, method, order, filter, enabled, metadata를 명시적으로 기록한다.
3. `application.yaml`을 독립적으로 읽어 catalog와 ordered equality를 검사한다.
4. Method가 없는 4개 route와 OpenAPI `SetPath`/`RewritePath`를 현재 동작 그대로 보존한다.

route 선택 계약과 요청 허용·인가 계약은 의미와 우선순위 규칙이 다르므로 하나의 필드로 합치지 않는다. 따라서 4B-1 catalog에는 allow/deny/access를 포함하지 않았다. 현재 감사에서는 Auth route와 교차하는 allow rule 73개를 확인했으며, `GatewayRequestPolicyFilter`는 일치 규칙의 존재를 검사하지만 `SecurityConfig`는 first-match로 접근 등급을 결정한다.

검증 gate:

- `./gradlew test --tests com.aandi.gateway.routing.AuthGatewayRouteCatalogTests --no-daemon`

#### 4B-2a: pure method/path evaluator 추출 — 완료

1. `GatewayRequestPolicyFilter`의 deny-before-allow method/path 판정을 side effect 없는 `MethodPathPolicyEvaluator`로 추출한다.
2. 기존 literal allow rule 254개와 deny rule 13개는 filter에 유지하고 evaluator로 판정만 위임해 runtime behavior를 보존한다.
3. allow/deny inventory, wildcard root/deep, nullable method와 기존 `ENDPOINT_NOT_ALLOWLISTED` 응답을 회귀 테스트로 고정한다.

검증 gate:

- allow rule 254개, deny rule 13개 inventory contract
- `./gradlew test --tests com.aandi.gateway.security.MethodPathPolicyEvaluatorTests --no-daemon`
- `./gradlew test bootJar --rerun-tasks --no-daemon`
- `git diff --check`

#### 4B-2b: Auth endpoint policy catalog — 완료

1. Auth 서비스 request matcher 73개를 ordered `EndpointPolicyContract`로 분리해 route catalog와 별도로 관리한다.
2. legacy/v1 28개, ping 2개, OpenAPI 2개, v2 41개의 선언 순서를 고정하고 `GatewayRequestPolicyFilter`가 catalog를 소비하도록 전환한다.
3. 전체 allow rule 254개를 Auth 73개와 비-Auth 181개로 분할해 합집합, 상호 배타성, 고유성, 기존 선언 순서를 회귀 테스트로 고정한다.
4. deny rule 13개와 deny-before-allow 평가, `SecurityConfig`의 접근 정책은 변경하지 않는다.

검증 gate:

- Auth 73개, 비-Auth 181개, 전체 allow 254개 inventory와 ordered fingerprint 일치
- Auth 그룹 크기 28/2/2/41 및 method 분포 일치
- `./gradlew test --tests com.aandi.gateway.security.MethodPathPolicyEvaluatorTests --no-daemon`
- `./gradlew test bootJar --rerun-tasks --no-daemon`
- `git diff --check`

#### 4B-2c: Auth access catalog dry-run — 완료

1. runtime source를 변경하지 않고 test source set에 ordered `AccessContract` 13개와 authenticated fallback을 추가한다.
2. permit-all, authenticated, role 집합을 분리하고 first-match 순서, 전역 OPTIONS 우선, exact/broad path 중첩을 현재 동작 그대로 기록한다.
3. Auth endpoint policy 73개의 접근 등급을 catalog에서 파생해 permit-all 12개, authenticated 21개, USER/ORGANIZER/ADMIN 11개, ORGANIZER/ADMIN 1개, ADMIN 28개로 고정한다.
4. `WebFilterChainProxy`만 직접 호출해 Gateway filter와 backend 영향을 제외하고 73개 전체를 anonymous, scope-only, USER, ORGANIZER, ADMIN principal로 검증한다.
5. `/v2/auth/me/password`의 authenticated fallback과 `/v1/me/password`, `/v2/me/password`의 역할 제한 비대칭을 동작 변경 없이 보존한다.
6. Method 미지정 Auth route 4개와 OPTIONS 동작을 교차 검증한다. Auth 73개는 모두 ROUTED이므로 정보가 없는 ROUTED/LOCAL 필드는 추가하지 않는다.

검증 gate:

- Auth 73개 ordered access projection과 접근 등급 분포 일치
- `./gradlew test --tests com.aandi.gateway.security.AuthAccessPolicyProjectionTests --tests com.aandi.gateway.security.AuthAccessPolicyProjectionIntegrationTests --no-daemon`
- `./gradlew test bootJar --rerun-tasks --no-daemon`
- `git diff --check`

이 단계에서 검증한 Auth 전용 matcher 13개와 별도 evaluator는 4B-2d-c에서 production 전역 catalog로 대체해 제거했다.

#### 4B-2d-a: global access catalog test-only dry-run — 완료

1. runtime source를 변경하지 않고 authenticated chain의 path matcher 37개와 최종 any-exchange 1개를 stable rule ID가 있는 전역 ordered catalog로 기록한다.
2. path 선언 105개, requirement별 rule/path 분포와 전체 선언 순서 fingerprint를 고정한다.
3. pure evaluator가 requirement와 winning rule ID를 함께 반환하도록 해 OPTIONS 우선, admin shadow, drafts/me narrow rule의 first-match를 보존한다.
4. 105개 path 전수, `/**` root/deep, method mismatch와 methodless matcher를 실제 `WebFilterChainProxy` 판정과 비교한다.
5. `gateway.auth.enabled=false`의 별도 permit-all chain과 JwtDecoder 비활성 계약을 독립 Spring context에서 검증한다.

검증 gate:

- ordered rule 38개, path rule 37개, path 선언 105개와 fingerprint 일치
- requirement rule/path 분포와 주요 winning rule ID 일치
- `./gradlew test --tests com.aandi.gateway.security.GlobalAccessPolicyCatalogTests --tests com.aandi.gateway.security.GlobalAccessPolicyCatalogIntegrationTests --tests com.aandi.gateway.security.PermitAllSecurityChainContractTests --no-daemon`
- `./gradlew test bootJar --rerun-tasks --no-daemon`
- `git diff --check`

#### 4B-2d-b: global access catalog runtime 소비 — 완료

1. 검증된 test-only catalog를 production source로 이동하고 `SecurityConfig`가 rule 순서대로 소비하도록 전환한다.
2. catalog는 path matcher와 any-exchange를 구분하고 permit-all, authenticated, 정확한 role 집합만 표현한다.
3. `SecurityConfig`의 예외 응답, JWT 변환, CORS와 conditional permit-all chain은 변경하지 않는다.
4. 전역 및 Auth access integration test로 접근 등급, rule 순서와 wildcard/method 경계의 동등성을 검증한다.

검증 gate:

- production catalog ordered rule 38개와 fingerprint 유지
- 전역 105개 path와 Auth 73개 projection의 live Security chain 동등성 유지
- `gateway.auth.enabled=false` permit-all chain 계약 유지
- `./gradlew test --tests com.aandi.gateway.security.GlobalAccessPolicyCatalogTests --tests com.aandi.gateway.security.GlobalAccessPolicyCatalogIntegrationTests --tests com.aandi.gateway.security.PermitAllSecurityChainContractTests --tests com.aandi.gateway.security.AuthAccessPolicyProjectionTests --tests com.aandi.gateway.security.AuthAccessPolicyProjectionIntegrationTests --no-daemon`
- `./gradlew test bootJar --rerun-tasks --no-daemon`
- `git diff --check`

#### 4B-2d-c: Auth access projection 중복 정리 — 완료

1. runtime 전환이 main에서 검증된 뒤 기존 Auth test-only matcher 목록을 제거하고 전역 catalog에서 Auth 73개 projection을 파생한다.
2. Auth ordered access fingerprint와 route ownership, `/v2/auth/me/password` fallback 비대칭 테스트는 유지한다.
3. 실제 handler나 route가 없는 legacy rule은 별도 사용처 확인과 테스트 후 제거한다.

검증 gate:

- Auth 전용 ordered matcher 13개와 별도 evaluator 제거
- 전역 catalog에서 파생한 Auth 73개 접근 등급 분포와 fingerprint 유지
- Auth route ownership, fallback 비대칭과 live Security chain 동등성 유지
- `./gradlew test --tests com.aandi.gateway.security.AuthAccessPolicyProjectionTests --tests com.aandi.gateway.security.AuthAccessPolicyProjectionIntegrationTests --tests com.aandi.gateway.security.GlobalAccessPolicyCatalogTests --tests com.aandi.gateway.security.GlobalAccessPolicyCatalogIntegrationTests --no-daemon`
- `./gradlew test bootJar --rerun-tasks --no-daemon`
- `git diff --check`

#### 4B-2e-a: Report endpoint policy catalog — 완료

1. Report allow rule 16개를 ordered `EndpointPolicyContract`로 분리한다.
2. OpenAPI 2개와 service path 14개의 기존 선언 위치를 유지해 전체 allow rule 순서를 보존한다.
3. method 분포, 고유성, 전체 254개 ordered fingerprint를 회귀 테스트로 고정한다.

검증 gate:

- Report 16개, 전체 allow 254개 inventory와 ordered fingerprint 일치
- `./gradlew test --tests com.aandi.gateway.security.MethodPathPolicyEvaluatorTests --no-daemon`
- `./gradlew test bootJar --rerun-tasks --no-daemon`
- `git diff --check`

#### 4B-2e-b: Online Judge endpoint policy catalog — 완료

1. Online Judge allow rule 20개를 ordered `EndpointPolicyContract`로 분리한다.
2. legacy 6개, OpenAPI 2개, v2 12개의 기존 선언 위치를 유지해 전체 allow rule 순서를 보존한다.
3. method 분포, 고유성, 전체 254개 ordered fingerprint를 회귀 테스트로 고정한다.

검증 gate:

- Online Judge 20개, 전체 allow 254개 inventory와 ordered fingerprint 일치
- `./gradlew test --tests com.aandi.gateway.security.MethodPathPolicyEvaluatorTests --no-daemon`
- `./gradlew test bootJar --rerun-tasks --no-daemon`
- `git diff --check`

경로 rewrite, 역할 인가, public/protected 구분은 각각 별도 변경으로 진행한다.

## 5단계: Monitor Bot 모듈 분리

분리 시작 시 큰 파일은 `assignment_ops.go` 1,568줄, `discord_format.go` 1,560줄, `interactions.go` 1,542줄이다. 먼저 함수 이동만 수행하고 로직 개선은 후속 단계로 미룬다.

### 5A: Discord Help formatting 분리 — 완료

1. `HelpText`, `HelpTextFor`와 topic, command, query helper 9개를 `help_text.go`로 이동한다.
2. exported signature, 도움말 문자열, 분기 순서와 `TruncateDiscordMessage` 호출은 변경하지 않는다.
3. `discord_format.go`를 1,560줄에서 1,101줄로 줄이고 Help 전용 파일 467줄로 분리한다.

검증 gate:

- 이동 전후 Help 함수 블록 byte diff 일치
- `go test ./internal/formatting`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5B: assignment formatting 분리 — 완료

1. assignment 응답 formatter 12개, 전용 타입 1개와 helper 4개를 `assignment_format.go`로 이동한다.
2. 공용 sanitize, truncate, row summary helper는 기존 파일에 유지한다.
3. 출력 문자열과 exported signature는 변경하지 않는다.
4. `discord_format.go`를 1,101줄에서 701줄로 줄이고 assignment 전용 파일 409줄로 분리한다.

검증 gate:

- 이동 전후 assignment formatter/type/helper 블록 byte diff 일치
- `go test ./internal/formatting`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-a: assignment issue lifecycle 분리 — 완료

1. issue 상태 적용, ack 판정과 이벤트 보강 함수 4개를 `assignment_issue_lifecycle.go`로 이동한다.
2. 여러 formatting 경로가 공유하는 severity와 key helper는 기존 파일에 유지한다.
3. `assignment_ops.go`를 1,568줄에서 1,431줄로 줄이고 lifecycle 전용 파일 145줄로 분리한다.

검증 gate:

- 이동 전후 assignment issue lifecycle 함수 블록 byte diff 일치
- `go test ./internal/monitor`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b1: dashboard command contract 고정 — 완료

1. dashboard watch의 channel fallback, scope와 interval 기본값을 handler 단위 테스트로 고정한다.
2. service watch, unwatch, status와 `/ops dashboard` dispatch의 controller 전달값을 고정한다.
3. 기본/서비스 view의 30분 window와 controller 부재, 잘못된 action/interval 응답을 runtime source 변경 없이 검증한다.

검증 gate:

- dashboard handler와 `/ops` dispatch 계약 테스트 통과
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b2: dashboard command family 분리 — 완료

1. dashboard 조회, watch, unwatch와 status handler 5개를 `interactions_dashboard.go`로 이동한다.
2. command schema, option parsing, controller 전달값, 응답 문자열과 timeout은 변경하지 않는다.
3. 이동 전후 함수 블록 byte diff와 5C-b1 계약 테스트를 모두 통과해야 한다.
4. `interactions.go`를 1,542줄에서 1,404줄로 줄이고 dashboard 전용 파일 150줄로 분리한다.

검증 gate:

- 이동 전후 dashboard command 함수 블록 byte diff 일치
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b3a: alert command contract 고정 — 완료

1. alert action, target, role과 channel fallback/override의 controller 전달값을 handler 단위 테스트로 고정한다.
2. `/ops alert` dispatch와 controller 부재 응답을 고정한다.
3. controller 오류의 sanitizing을 runtime source 변경 없이 검증한다.

검증 gate:

- alert handler와 `/ops` dispatch 계약 테스트 통과
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b3b: alert command family 분리 — 완료

1. 활성 alert handler를 `interactions_alert.go`로 이동한다.
2. command schema, option parsing, controller 전달값과 응답 문자열은 변경하지 않는다.
3. 이동 전후 함수 블록 byte diff와 5C-b3a 계약 테스트를 모두 통과해야 한다.
4. `interactions.go`를 1,404줄에서 1,389줄로 줄이고 alert 전용 파일 22줄로 분리한다.

### 5C-b4a: logs watch command contract 고정 — 완료

1. logs watch의 channel fallback, service/mode/since/interval/limit 기본값과 limit 상한을 handler 단위 테스트로 고정한다.
2. 명시적 option 전달, unwatch/watches와 `/ops logs` dispatch의 controller 호출을 고정한다.
3. controller 부재, 잘못된 interval과 오류 sanitizing을 runtime source 변경 없이 검증한다.

검증 gate:

- logs watch/unwatch/watches handler와 `/ops` dispatch 계약 테스트 통과
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b4b: logs watch command family 분리 — 완료

1. logs watch, unwatch, watches handler 3개를 전용 파일로 이동한다.
2. command schema, option parsing, controller 전달값, 응답 문자열과 sanitizing은 변경하지 않는다.
3. 이동 전후 함수 블록 byte diff와 5C-b4a 계약 테스트를 모두 통과해야 한다.

검증 gate:

- 이동 전후 logs watch/unwatch/watches 함수 블록 byte diff 일치
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b5a: dashboard service command contract 고정 — 완료

1. service validation, V2 로그 미연동 호환 응답과 잘못된 view 응답을 직접 handler 테스트로 고정한다.
2. health view의 service 상태와 후속 logs command를 고정한다.
3. 기존 dashboard service summary의 30분 기본 window 계약을 유지한다.

검증 gate:

- dashboard service handler와 기존 dashboard dispatch 계약 테스트 통과
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b5b: dashboard service command 분리 — 완료

1. dashboard service handler를 dashboard command 전용 파일로 이동한다.
2. service normalization, view 분기, 응답 문자열과 후속 command는 변경하지 않는다.
3. 이동 전후 함수 블록 byte diff와 5C-b5a 계약 테스트를 모두 통과해야 한다.

검증 gate:

- 이동 전후 dashboard service 함수 블록 byte diff 일치
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b6a: assignment controller action contract 고정 — 완료

1. assignment events, ack와 unack의 controller 전달값을 직접 handler 테스트로 고정한다.
2. identifier validation, ack reason 필수, controller 부재와 오류 sanitizing을 고정한다.
3. Web Admin API와 CloudWatch 조회 경로는 후속 assignment 조회 계약에서 별도로 다룬다.

검증 gate:

- assignment events/ack/unack handler 계약 테스트 통과
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b6b: assignment controller action 분리 — 완료

1. assignment command의 controller action 분기를 전용 helper와 파일로 이동한다.
2. validation 순서, controller 전달값, 응답 문자열과 sanitizing은 변경하지 않는다.
3. 5C-b6a 계약 테스트와 전체 race, vet를 통과해야 한다.

검증 gate:

- assignment action 계약 테스트 통과
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b7a: assignment query contract 고정 — 완료

1. 실제 test HTTP server와 `reportadmin.Client`로 list, summary, check, submissions 성공 경로를 handler 수준에서 고정한다.
2. token refresh와 각 Web Admin API endpoint 선택을 검증한다.
3. 각 조회 결과가 Discord formatting 응답까지 이어지는지 검증한다.

검증 gate:

- assignment query handler와 Admin API endpoint 계약 테스트 통과
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b7b: assignment query command family 분리 — 완료

1. assignment list, summary, check, submissions와 조회 전용 fallback/helper를 전용 파일군으로 이동한다.
2. Admin API endpoint, validation, fallback, formatting과 응답 문자열은 변경하지 않는다.
3. 이동 전후 함수 블록 diff와 5C-b7a 계약 테스트를 모두 통과해야 한다.

4. `interactions.go`를 1,292줄에서 981줄로 줄이고 assignment query 전용 파일 323줄로 분리한다.

검증 gate:

- 이동 전후 assignment query 함수 블록 일치
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b8a: logs query command contract 고정 — 완료

1. recent, errors, critical, top, slow, security와 trace mode의 query routing과 후속 command를 handler 테스트로 고정한다.
2. action/service/mode validation, V2 미연동 응답, all-service guard와 events service 제한을 고정한다.
3. mode 미지정 trace query inference와 trace query 필수 조건을 고정한다.

검증 gate:

- logs query mode와 guard 계약 테스트 통과
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b8b: logs query command family 분리 — 완료

1. logs query dispatcher와 recent/errors/top/slow/security/trace 전용 handler/helper를 파일군으로 이동한다.
2. query builder, validation, guard, follow-up command와 formatting은 변경하지 않는다.
3. 이동 전후 함수 블록 diff와 5C-b8a 계약 테스트를 모두 통과해야 한다.

4. `interactions.go`를 981줄에서 572줄로 줄이고 logs query 전용 파일 421줄로 분리한다.

검증 gate:

- 이동 전후 logs query 함수 블록 일치
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5C-b9: remaining interaction command family 감사 — 완료

1. 남은 활성 `/ops` command는 직접 handler 계약을 먼저 고정한 뒤 family 단위로 이동한다.
2. deprecated compatibility 응답과 호출처가 없는 legacy helper 삭제는 별도 사용처 감사로 분리한다.
3. 각 family는 독립 PR에서 targeted, 전체 race, vet와 byte diff를 통과해야 한다.

활성 `/ops` command family는 dashboard, alert, logs watch, logs query, assignment action과 assignment query 파일로 분리했다. `interactions.go`에는 transport/dispatch와 공용 option helper를 유지하고, 호출처가 없는 legacy handler는 6단계 삭제 감사 대상으로 넘긴다.

### 5D: assignment diagnosis와 snapshot 계산 분리 — 완료

1. course/assignment snapshot, detail merge와 변화 event 계산을 `assignment_diagnosis.go`로 이동한다.
2. diagnosis, evidence, issue key/fingerprint와 assignment 시간/status 판정 helper를 함께 이동한다.
3. 기존 diagnosis, lifecycle, evidence와 assignment ops 통합 테스트를 변경 없이 유지한다.
4. `assignment_ops.go`를 1,431줄에서 1,062줄로 줄이고 diagnosis 전용 파일 379줄로 분리한다.

검증 gate:

- `go test ./internal/monitor`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5E: assignment dashboard와 event formatting 분리 — 완료

1. assignment dashboard, recent event group, issue digest와 event formatting을 전용 파일로 이동한다.
2. 문자열, 정렬, grouping, limit과 next command는 변경하지 않는다.
3. 기존 dashboard/grouping/notification 테스트와 전체 race, vet를 통과해야 한다.

4. `assignment_ops.go`를 1,062줄에서 581줄로 줄이고 dashboard/event formatting 전용 파일 494줄로 분리한다.

검증 gate:

- `go test ./internal/monitor`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 5F: assignment issue 조회와 ack operation 분리 — 완료

1. diagnosis description, issue history/status와 ack/unack operation을 전용 파일로 이동한다.
2. state 조회, validation, 정렬, 응답 문자열과 persistence는 변경하지 않는다.
3. 기존 ack/history/diagnosis 테스트와 전체 race, vet를 통과해야 한다.

4. `assignment_ops.go`를 581줄에서 289줄로 줄이고 issue operation 전용 파일 302줄로 분리한다.

검증 gate:

- `go test ./internal/monitor`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

## 6단계: 레거시 정리

### 6A: 호출처 없는 Discord handler 제거 — 완료

1. `opsAlarmsCommand`, `countCommand`, `assignmentEventsCommand`, `assignmentAckCommand`, `assignmentUnackCommand`, `retentionCommand`가 정의 외 호출처가 없음을 전수 검색으로 확인한다.
2. dead `opsAlarmsCommand`에서만 사용한 `filterAlarmNames`를 함께 제거한다.
3. 활성 transport/dispatch, `serviceHasAlarm`과 공용 option helper는 유지한다.
4. `interactions.go`를 572줄에서 430줄로 줄인다.

검증 gate:

- 제거 symbol의 Go source 사용처 0건
- `go test ./internal/discord`
- `go test -race ./...`
- `go vet ./...`
- `git diff --check`

### 6B: 환경 변수와 문서 drift 검사 — 완료

1. Gateway `application.yaml` placeholder와 Monitor Bot config의 `env*` 호출에서 runtime key를 추출한다.
2. Spring relaxed binding으로 소비하는 `GATEWAY_AUTH_ENABLED`를 명시적 alias로 포함한다.
3. `.env.example`의 주석 처리된 runtime override를 포함해 누락, 초과와 중복 key를 검사한다.
4. Gateway 33개, Monitor Bot 45개와 `.env.example` 77개 key의 합집합 동등성을 CI에서 검증한다.

검증 gate:

- `python3 scripts/validate-env-contract.py`
- `.github/workflows/ci.yml` YAML parse
- `git diff --check`

### 6C: 완료 문서 archive와 metrics drift 정리 — 완료

1. 측정 문서 참조와 상태를 감사한 결과 `cicd-next-measurement-plan.md`의 Phase 3~5가 남아 있어 archive하지 않는다.
2. `cicd-optimization.md`, `cicd-measurement-audit.md`, `resume-metrics.md`는 README와 consistency validator가 참조하는 활성 문서로 유지한다.
3. metrics JSON의 안전 flag, 표본 수, 실패 수와 improvement 조건 검사를 CI에 연결한다.
4. metrics JSON과 optimization/audit/resume 문서의 수치·사용 가능 여부 동등성 검사를 CI에 연결한다.

검증 gate:

- `python3 scripts/validate-gateway-cicd-metrics.py docs/metrics/gateway-cicd-remeasure.json`
- `python3 scripts/validate-doc-metrics-consistency.py --metrics docs/metrics/gateway-cicd-remeasure.json --docs docs/cicd-optimization.md docs/resume-metrics.md docs/cicd-measurement-audit.md`
- `.github/workflows/ci.yml` YAML parse
- `git diff --check`

### 6D: Kotlin/Jackson deprecation 제거 — 완료

1. Jackson 3에서 deprecated된 `JsonNode.asText`를 동일 의미의 `asString`으로 교체한다.
2. deprecated `isTextual`을 `isString`으로 교체한다.
3. timestamp default, error fallback과 JSON 문자열/숫자/boolean/null 변환 계약을 유지한다.

검증 gate:

- `./gradlew test --tests com.aandi.gateway.logging.ApiLoggingContractTests --no-daemon`
- `./gradlew test bootJar --rerun-tasks --no-daemon`
- Kotlin compile deprecation warning 0건
- `git diff --check`

삭제는 `rg` 사용처 확인과 clean clone 검증을 모두 통과한 항목에만 적용한다.
