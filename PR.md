# Gateway 서버 초기 설정

## Summary

- Spring Cloud Gateway (WebFlux) 기반 게이트웨이 서버 초기 구성
- Redis Reactive 연결 설정 및 `ReactiveRedisTemplate` Bean 등록
- Spring Security 의존성 제거 (인증/인가 불필요)
- 로컬 개발용 `docker-compose.yml` 구성 (gateway + redis)
- GitHub Actions CI/CD 파이프라인 구성

## Changes

**설정**
- `application.yaml` — Redis(Lettuce), Gateway 타임아웃, Actuator 엔드포인트 설정
- `build.gradle.kts` — Spring Security 의존성 제거

**소스**
- `RedisConfig.kt` — `ReactiveRedisTemplate<String, String>` Bean 등록

**인프라**
- `Dockerfile` — `eclipse-temurin:21-jre-alpine` 기반 이미지
- `docker-compose.yml` — gateway + redis 로컬 실행 환경

**CI/CD**
- `ci.yml` — `main` / `develop` 브랜치 push·PR 시 테스트 및 빌드
- `cd.yml` — `vX.Y.Z` 태그 push 시 `ghcr.io` Docker 이미지 빌드·푸시

## Test plan

- [ ] `./gradlew test` 통과 확인
- [ ] `docker-compose up` 후 `GET /actuator/health` 응답 확인
- [ ] `GET /actuator/gateway/routes` 빈 배열 반환 확인
