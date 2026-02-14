# aandi-gateway-server 설정 계획

## 프로젝트 개요

- **프레임워크**: Spring Boot 4.0.2 + Spring Cloud Gateway (WebFlux)
- **언어**: Kotlin 2.2.21 / Java 21
- **목적**: 하위 서비스 라우팅을 위한 API Gateway (현재는 라우트 등록 구조만 세팅)
- **제외 항목**: 보안/인증/인가 (Spring Security 의존성 제거)

---

## 작업 목록

### 1. Security 의존성 제거

**파일**: `build.gradle.kts`

Spring Security가 클래스패스에 존재하면 Spring Boot가 자동으로 Basic Auth를 활성화해
인증 없이 모든 요청을 차단한다. 보안 인증이 불필요하므로 관련 의존성을 제거한다.

**제거 대상**:
- `spring-boot-starter-security`
- `spring-boot-starter-security-test`

---

### 2. Redis 설정

**파일**: `src/main/resources/application.yaml`
**파일**: `src/main/kotlin/com/aandi/gateway/config/RedisConfig.kt`

Spring Data Redis Reactive를 사용해 Lettuce 클라이언트로 Redis에 연결한다.
환경변수로 host/port/password를 주입받아 환경별 유연성을 확보한다.

**설정 항목**:
| 항목 | 기본값 | 환경변수 |
|------|--------|----------|
| host | localhost | `REDIS_HOST` |
| port | 6379 | `REDIS_PORT` |
| password | (없음) | `REDIS_PASSWORD` |
| connection timeout | 2000ms | - |
| Lettuce pool max-active | 8 | - |

**RedisConfig.kt 역할**:
- `ReactiveRedisTemplate<String, String>` Bean 등록
- `StringRedisSerializer` 직렬화 설정

---

### 3. Gateway 설정

**파일**: `src/main/resources/application.yaml`

Spring Cloud Gateway의 기본 구조를 잡아 라우트 추가가 쉽도록 준비한다.
현재는 연결된 하위 서비스가 없으므로 **라우트는 빈 배열**로 두고, 나중에 추가하는 방법을 주석으로 안내한다.

**설정 항목**:
- `spring.cloud.gateway.routes`: [] (빈 상태 유지, 추가 예시 주석 포함)
- HTTP 클라이언트 타임아웃: connect 3s / response 10s
- Actuator Gateway 엔드포인트 활성화 (`/actuator/gateway`)
- 헬스체크 엔드포인트 활성화 (`/actuator/health`)

**라우트 추가 방법 (나중에 서비스 연결 시)**:
```yaml
spring:
  cloud:
    gateway:
      routes:
        - id: example-service
          uri: http://example-service:8080
          predicates:
            - Path=/api/example/**
          filters:
            - StripPrefix=1
```

---

### 4. Dockerfile

**파일**: `Dockerfile`

멀티스테이지 빌드 없이 단순하게 JAR를 실행하는 이미지를 작성한다.
CI/CD에서 Gradle로 빌드 후 생성된 JAR를 Docker 이미지로 패키징한다.

```
eclipse-temurin:21-jre-alpine → /app/app.jar → EXPOSE 8080
```

---

### 5. docker-compose.yml (로컬 개발용)

**파일**: `docker-compose.yml`

로컬 개발 환경에서 Redis와 Gateway를 함께 실행하기 위한 Compose 파일.

**서비스 구성**:
- `redis`: redis:7-alpine, 포트 6379
- `gateway`: 현재 프로젝트 빌드 이미지, 포트 8080, Redis에 연결

---

### 6. GitHub Actions CI

**파일**: `.github/workflows/ci.yml`

**트리거**: `main`, `develop` 브랜치 push 및 PR

**단계**:
1. Checkout
2. Java 21 (Temurin) 설정
3. Gradle 캐시 설정
4. `./gradlew test` — 테스트 실행
5. `./gradlew build` — JAR 빌드
6. 테스트 결과 리포트 업로드 (선택)

---

### 7. GitHub Actions CD

**파일**: `.github/workflows/cd.yml`

**트리거**: `main` 브랜치 push (CI 성공 후)

**단계**:
1. Checkout
2. Java 21 설정 + Gradle 캐시
3. `./gradlew build -x test` — JAR 빌드 (테스트는 CI에서 완료)
4. GitHub Container Registry (ghcr.io) 로그인
5. Docker 이미지 빌드 및 푸시
   - 태그: `ghcr.io/<owner>/<repo>:latest` + `:<sha>`

---

## 생성/수정 파일 목록

```
aandi-gateway-server/
├── PLAN.md                                          ← 이 파일
├── build.gradle.kts                                 ← Security 의존성 제거
├── Dockerfile                                       ← 신규
├── docker-compose.yml                               ← 신규
├── .github/
│   └── workflows/
│       ├── ci.yml                                   ← 신규
│       └── cd.yml                                   ← 신규
└── src/
    └── main/
        ├── kotlin/com/aandi/gateway/
        │   └── config/
        │       └── RedisConfig.kt                   ← 신규
        └── resources/
            └── application.yaml                     ← Redis + Gateway 설정 추가
```

---

## 실행 방법

### 로컬 (docker-compose)
```bash
./gradlew build -x test
docker-compose up -d
```

### 헬스체크 확인
```bash
curl http://localhost:8080/actuator/health
curl http://localhost:8080/actuator/gateway/routes
```
