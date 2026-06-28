# Gateway CI/CD Metrics

> GitHub Actions run metadata를 읽기 전용으로 조회해 계산한 값입니다.
> workflow_dispatch, tag push, AWS CLI, SSH, docker push, 운영 URL 접근은 수행하지 않습니다.

## Scope

- Repository: `Team-AnI/A-AND-I-GATEWAY-SERVER`
- Generated at: `2026-06-28T02:00:13Z`
- Source commands: `gh run list`, `gh run view --json jobs`
- Secrets, AWS host, EC2 host, SSH key, token 값은 저장하지 않습니다.
- 3회 미만의 수치는 이력서 사용 비추천으로 표시합니다.

## CI

- Workflow: `CI`
- Branch: `main`
- Runs analyzed: `20`

| Workflow | Metric | Runs | Missing | Avg | Median | Min | Max | Failure rate | 신뢰도 | 사용 여부 |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :--- | :--- |
| CI | CI 전체 시간 | 20 | 0 | 2m 29s | 2m 28s | 1m 58s | 3m 9s | 0.00% | 확인 완료 | 사용 가능 |
| CI | ./gradlew test | 20 | 0 | 1m 43s | 1m 42s | 1m 23s | 2m 6s | 0.00% | 확인 완료 | 사용 가능 |
| CI | monitor-bot go test | 16 | 4 | 30.6s | 30.0s | 24.0s | 38.0s | 0.00% | 확인 완료 | 사용 가능 |
| CI | ./gradlew build 또는 build -x test | 20 | 0 | 2.5s | 2.0s | 2.0s | 3.0s | 0.00% | 확인 완료 | 사용 가능 |
| CI | k6 install | 7 | 13 | 1.0s | 1.0s | 1.0s | 1.0s | 0.00% | 확인 완료 | 사용 가능 |
| CI | performance asset validation | 7 | 13 | 2.7s | 2.0s | 1.0s | 7.0s | 0.00% | 확인 완료 | 사용 가능 |

## CD

- Workflow: `CD`
- Branch: `[all]`
- Runs analyzed: `20`
- Deploy step은 과거 run metadata의 step duration만 조회했습니다. 실행, SSH 접속, AWS 명령은 수행하지 않았습니다.

| Workflow | Metric | Runs | Missing | Avg | Median | Min | Max | Failure rate | 신뢰도 | 사용 여부 |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | :--- | :--- |
| CD | CD 전체 시간 | 20 | 0 | 633m 33s | 4m 18s | 49.0s | 12591m 59s | 5.00% | 확인 완료 | 사용 가능 |
| CD | ./gradlew test | 20 | 0 | 1m 35s | 1m 37s | 34.0s | 1m 55s | 5.00% | 확인 완료 | 사용 가능 |
| CD | monitor-bot go test | 14 | 6 | 27.9s | 30.0s | 0.0s | 36.0s | 7.14% | 확인 완료 | 사용 가능 |
| CD | ./gradlew build | 20 | 0 | 2.5s | 3.0s | 0.0s | 3.0s | 5.00% | 확인 완료 | 사용 가능 |
| CD | Gateway Docker build/push | 20 | 0 | 23.6s | 25.0s | 0.0s | 29.0s | 5.00% | 확인 완료 | 사용 가능 |
| CD | monitor-bot Docker build/push | 14 | 6 | 35.8s | 38.0s | 0.0s | 45.0s | 7.14% | 확인 완료 | 사용 가능 |
| CD | Deploy step 조회 전용 | 20 | 0 | 39.6s | 43.0s | 0.0s | 51.0s | 5.00% | 확인 완료 | 참고용 |

## Resume Sentence Candidate

- GitHub Actions에서 Gateway 테스트, Monitor Bot 테스트, JAR 빌드, k6 asset 검증을 자동화하고 CI median 2m 28s 기준으로 검증 시간을 관리

## Notes

- GitHub Actions UI와 GitHub CLI가 제공하는 step startedAt/completedAt 기준입니다.
- CD 전체 시간은 GitHub run createdAt/updatedAt metadata 기준입니다. 오래 열린 run이 있으면 평균보다 중앙값과 step별 시간을 우선 확인합니다.
- Docker build/push와 deploy 시간은 과거 CD run metadata로만 계산합니다.
- 운영 최대 처리량, 운영 배포 안정성, 운영 트래픽 처리량으로 해석하지 않습니다.
