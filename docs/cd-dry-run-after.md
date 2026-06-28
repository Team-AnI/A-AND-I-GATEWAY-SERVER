# CD Dry Run Metrics

- Repo: `Team-AnI/A-AND-I-GATEWAY-SERVER`
- Branch: `develop-cicd-parallelization-metrics`
- Successful Runs: 5
- Failure Rate: 0.0
- Confidence: 확인 완료
- Resume Use: 사용 가능

## Workflow

| Metric | Median | Average | Min | Max | Count |
| --- | ---: | ---: | ---: | ---: | ---: |
| Duration | 2m 29s | 2m 31s | 2m 25s | 2m 42s | 5 |

## Jobs

| Job | Median | Average | Min | Max | Count |
| --- | ---: | ---: | ---: | ---: | ---: |
| backend-test | 1m 57s | 1m 59s | 1m 51s | 2m 14s | 5 |
| cd-dry-run-summary | 4s | 4s | 3s | 6s | 5 |
| gateway-image-build-dry-run | 19s | 20s | 15s | 26s | 5 |
| monitor-bot-image-build-dry-run | 44s | 45s | 40s | 53s | 5 |
| monitor-bot-test | 11s | 11s | 9s | 14s | 5 |

## Steps

| Step | Median | Average | Min | Max | Count |
| --- | ---: | ---: | ---: | ---: | ---: |
| Build JAR for image dry run | 3s | 3s | 2s | 3s | 5 |
| Checkout | 1s | 1s | 0s | 2s | 20 |
| Complete job | 0s | 0s | 0s | 1s | 25 |
| Download JAR | 2s | 2s | 1s | 4s | 5 |
| Gateway image dry-run build | 4s | 5s | 2s | 9s | 5 |
| Monitor Bot image dry-run build | 31s | 32s | 30s | 39s | 5 |
| Post Checkout | 0s | 0s | 0s | 1s | 20 |
| Post Gateway image dry-run build | 1s | 1s | 1s | 2s | 5 |
| Post Monitor Bot image dry-run build | 1s | 1s | 1s | 1s | 5 |
| Post Set up Docker Buildx | 1s | 1s | 0s | 2s | 10 |
| Post Setup Go | 0s | 0s | 0s | 1s | 5 |
| Post Setup Gradle | 0s | 0s | 0s | 0s | 5 |
| Post Setup Java 21 | 0s | 0s | 0s | 0s | 5 |
| Run backend tests | 1m 41s | 1m 44s | 1m 38s | 1m 56s | 5 |
| Run monitor-bot tests | 4s | 4s | 3s | 5s | 5 |
| Set up Docker Buildx | 6s | 6s | 4s | 9s | 10 |
| Set up job | 1s | 1s | 0s | 3s | 25 |
| Setup Go | 2s | 2s | 1s | 3s | 5 |
| Setup Gradle | 2s | 1s | 0s | 2s | 5 |
| Setup Java 21 | 1s | 1s | 0s | 1s | 5 |
| Summarize CD dry-run jobs | 0s | 0s | 0s | 0s | 5 |
| Upload CD dry-run summary | 1s | 1s | 0s | 1s | 5 |
| Upload JAR | 3s | 3s | 3s | 4s | 5 |
