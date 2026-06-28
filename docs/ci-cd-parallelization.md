# Gateway CI Parallelization Metrics

> 이 문서는 GitHub Actions CI 검증 시간 개선만 다룹니다.
>
> 운영 배포 시간, 운영 트래픽 처리량, 서비스 성능 개선 수치가 아닙니다.

## 변경 내용

기존 CI는 단일 `build` job 안에서 Gradle test, monitor-bot Go test, JAR build, k6 설치와 performance asset validation을 순차 실행했습니다.

이번 변경은 서로 독립적인 검증을 다음 job으로 분리해 병렬 실행합니다.

| Job | 역할 |
| :--- | :--- |
| `gateway-tests` | `./gradlew test` |
| `monitor-bot-tests` | `cd monitor-bot && go test ./...` |
| `jar-build` | `./gradlew build -x test` |
| `performance-assets` | k6 설치, performance asset validation |
| `build` | 기존 required check 이름을 유지하는 aggregate job |

`build` job은 네 개 job 결과가 모두 `success`일 때만 통과합니다.

## Before/After

GitHub Actions metadata를 읽기 전용으로 조회했습니다.

| 구분 | Run | Branch | Commit | Event | Conclusion | 전체 시간 |
| :--- | :--- | :--- | :--- | :--- | :--- | ---: |
| Before | [`27965838063`](https://github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/actions/runs/27965838063) | `main` | `9c3bcf803fc653df84fd42b62ad8dda5d0967380` | `push` | `success` | 177초 |
| After | [`28310185706`](https://github.com/Team-AnI/A-AND-I-GATEWAY-SERVER/actions/runs/28310185706) | `develop-ci-parallel-timing` | `d66c2f06d6a395cd4c3166faf0ca3ea0f82a5ac6` | `pull_request` | `success` | 129초 |

단일 run 기준 전체 CI 시간은 177초에서 129초로 48초 감소했습니다.

| 지표 | 값 |
| :--- | ---: |
| 단축 시간 | 48초 |
| 단축률 | 27.1% |
| Before 전체 시간 | 2분 57초 |
| After 전체 시간 | 2분 9초 |

## Job Detail

### Before

| Step | 시간 |
| :--- | ---: |
| Run tests | 112초 |
| Run monitor-bot tests | 38초 |
| Build JAR | 3초 |
| Install k6 | 1초 |
| Validate performance assets | 4초 |
| 단일 `build` job 전체 | 173초 |

### After

| Job | 시간 |
| :--- | ---: |
| `gateway-tests` | 118초 |
| `jar-build` | 93초 |
| `monitor-bot-tests` | 38초 |
| `performance-assets` | 9초 |
| aggregate `build` | 4초 |

After run의 전체 시간은 가장 긴 `gateway-tests` job과 aggregate `build` job에 수렴했습니다.

## Safety

- CD workflow를 실행하지 않았습니다.
- docker push, ECR push, EC2 SSH, AWS CLI, 운영 배포를 실행하지 않았습니다.
- GitHub Actions metadata는 read-only로 조회했습니다.
- 운영 secret, 운영 로그, 운영 사용자 데이터는 저장하지 않았습니다.

## Resume Sentence

GitHub Actions CI를 Gateway 테스트, Monitor Bot 테스트, JAR 빌드, k6 asset 검증 job으로 병렬화해 단일 run 기준 검증 시간을 177초에서 129초로 27.1% 단축

단일 run 기준이므로 이력서에는 “단일 GitHub Actions run 기준” 또는 “GitHub Actions metadata 기준”을 함께 적습니다.
