# Commit Unit Plan

기준 문서:
- `docs/COMMIT_CONVENTION.md`

## 커밋 순서

1. `feat(security): add jwt security and principal headers`
- 범위:
  - `build.gradle.kts` (security dependency)
  - `src/main/resources/application.yaml` (oauth2 resource server)
  - `src/main/kotlin/com/aandi/gateway/common/HeaderNames.kt`
  - `src/main/kotlin/com/aandi/gateway/security/*`
  - `src/test/kotlin/com/aandi/gateway/security/*`
- 목적:
  - JWT 인증 필수 정책
  - 내부 인증 헤더 sanitize + 재주입
  - 보안 테스트 추가

2. `feat(cache): add redis token-context cache filter`
- 범위:
  - `src/main/kotlin/com/aandi/gateway/cache/*`
  - `src/main/resources/application.yaml` (`app.token-cache.ttl`)
  - `src/test/kotlin/com/aandi/gateway/cache/*`
- 목적:
  - 인증 사용자 컨텍스트 24시간 Redis 캐시
  - HIT/MISS 헤더 전달
  - Redis 장애 시 graceful degrade

3. `chore(build): harden cd pipeline and docker artifact`
- 범위:
  - `.github/workflows/cd.yml`
  - `build.gradle.kts` (bootJar/jar task)
  - `Dockerfile`
  - `src/main/resources/application.yaml` (actuator exposure 축소)
- 목적:
  - CD에서 테스트 생략 제거
  - 실행 jar 파일명 고정(app.jar)
  - actuator 노출 최소화

4. `docs(gateway): add handover package and api docs`
- 범위:
  - `docs/COMMIT_CONVENTION.md`
  - `docs/PR_GUIDE.md`
  - `docs/COMMIT_UNIT_PLAN.md`
  - `docs/GATEWAY_HANDOVER.md`
  - `docs/PACKAGE_STRATEGY.md`
  - `docs/API_SPEC.md`
- 목적:
  - 인수인계/코딩규칙/패키지전략/API 계약 문서화
