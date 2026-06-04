# Gateway Docs

> 메인 README로 돌아가기: [README](../README.md)

이 문서는 A-AND-I-GATEWAY-SERVER를 포트폴리오/이력서 증빙용으로 읽을 때 필요한 상세 근거를 모은 목차입니다. 루트 README는 3분 안에 역할을 파악하는 랜딩 페이지이고, 이 디렉터리는 코드 근거, 운영 흐름, 테스트 결과, 배포 기준을 설명합니다.

## 추천 읽기 순서

1. [Architecture](./architecture.md)
2. [Observability](./observability.md)
3. [Discord Monitor Bot](./discord-monitor-bot.md)
4. [Error Response Policy](./error-response-policy.md)
5. [Test Results](./test.md)
6. [Resume Evidence](./resume-evidence.md)

## 상세 문서

| 문서 | 설명 |
| :--- | :--- |
| [architecture.md](./architecture.md) | Gateway, downstream service, Redis, monitor-bot, CloudWatch Logs의 구조 |
| [observability.md](./observability.md) | 장애 감지 문제, CloudWatch Logs, alert routing, trace drilldown 흐름 |
| [discord-monitor-bot.md](./discord-monitor-bot.md) | Go HTTP Interactions sidecar, read-only 원칙, command family, alert routing |
| [error-response-policy.md](./error-response-policy.md) | 공통 응답 구조, 5자리 error code, HTTP status mapping |
| [deployment.md](./deployment.md) | CI/CD, Docker, nginx, ECR/EC2 배포 구조 |
| [test.md](./test.md) | JVM/Go 테스트 실행 결과와 coverage 확인 |
| [performance-measurement.md](./performance-measurement.md) | 현재 측정값 유무와 향후 측정 기준 |
| [query-tuning.md](./query-tuning.md) | CloudWatch query builder coverage 보강과 실제 query tuning 미적용 사유 |
| [resume-evidence.md](./resume-evidence.md) | 이력서 문장과 코드/테스트/문서 근거 연결 |
| [demo-capture.md](./demo-capture.md) | GIF/이미지 생성 상태와 수동 촬영 기준 |

## API / 운영 흐름

| 문서 | 설명 |
| :--- | :--- |
| [api-flows/README.md](./api-flows/README.md) | API/운영 흐름 문서 목차 |
| [api-flows/ops-alert.md](./api-flows/ops-alert.md) | CloudWatch Logs 기반 critical/general alert routing |
| [api-flows/trace-drilldown.md](./api-flows/trace-drilldown.md) | traceId 기반 로그 추적 흐름 |
| [api-flows/assignment-audit.md](./api-flows/assignment-audit.md) | Report EVENT 로그 기반 assignment audit feed |

## Troubleshooting

| 문서 | 설명 |
| :--- | :--- |
| [troubleshooting/README.md](./troubleshooting/README.md) | 문제 해결 문서 목차 |
| [troubleshooting/discord-interaction.md](./troubleshooting/discord-interaction.md) | Discord command 등록/서명/권한 문제 점검 |
| [troubleshooting/critical-alert-routing.md](./troubleshooting/critical-alert-routing.md) | critical channel과 role mention 문제 점검 |

## 기존 운영 문서

| 문서 | 설명 |
| :--- | :--- |
| [ENV_SETUP.md](./ENV_SETUP.md) | EC2 환경변수와 health check 기준 |
| [GATEWAY_ERROR_CODES.md](./GATEWAY_ERROR_CODES.md) | 기존 Gateway error code mapping |
| [GATEWAY_HANDOVER.md](./GATEWAY_HANDOVER.md) | Gateway 운영 인수인계 |
| [log-retention.md](./log-retention.md) | CloudWatch Logs retention과 Docker log rotation |
| [docker-cleanup-systemd.md](./docker-cleanup-systemd.md) | Docker cleanup systemd timer 기준 |
