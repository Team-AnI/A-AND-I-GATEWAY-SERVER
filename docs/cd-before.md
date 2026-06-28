# CD Metrics

- Repo: `Team-AnI/A-AND-I-GATEWAY-SERVER`
- Branch: `all-refs`
- Successful Runs: 19
- Failure Rate: 0.05
- Confidence: 확인 완료
- Resume Use: 사용 비추천

## Workflow

| Metric | Median | Average | Min | Max | Count |
| --- | ---: | ---: | ---: | ---: | ---: |
| Duration | 4m 18s | 666m 51s | 2m 55s | 12591m 59s | 19 |

## Jobs

| Job | Median | Average | Min | Max | Count |
| --- | ---: | ---: | ---: | ---: | ---: |
| build-push-deploy | 4m 13s | 3m 53s | 2m 47s | 4m 47s | 19 |

## Steps

| Step | Median | Average | Min | Max | Count |
| --- | ---: | ---: | ---: | ---: | ---: |
| Build JAR | 3s | 3s | 2s | 3s | 19 |
| Build and push Docker image | 25s | 25s | 21s | 29s | 19 |
| Build and push monitor-bot Docker image | 38s | 39s | 36s | 45s | 13 |
| Checkout | 1s | 1s | 0s | 1s | 19 |
| Complete job | 0s | 0s | 0s | 1s | 19 |
| Configure AWS credentials | 1s | 1s | 1s | 3s | 19 |
| Deploy to EC2 via SSH | 43s | 42s | 31s | 51s | 19 |
| Login to Amazon ECR | 3s | 2s | 1s | 3s | 19 |
| Post Build and push Docker image | 1s | 1s | 1s | 2s | 19 |
| Post Build and push monitor-bot Docker image | 1s | 1s | 0s | 2s | 13 |
| Post Checkout | 0s | 0s | 0s | 1s | 19 |
| Post Configure AWS credentials | 0s | 0s | 0s | 1s | 19 |
| Post Login to Amazon ECR | 0s | 0s | 0s | 1s | 19 |
| Post Setup Go | 0s | 0s | 0s | 1s | 13 |
| Post Setup Gradle | 0s | 0s | 0s | 1s | 19 |
| Post Setup Java 21 | 0s | 0s | 0s | 1s | 19 |
| Run tests | 1m 37s | 1m 38s | 1m 30s | 1m 55s | 19 |
| Set up job | 5s | 5s | 3s | 8s | 19 |
| Setup Go | 1s | 1s | 0s | 3s | 13 |
| Setup Gradle | 1s | 1s | 0s | 2s | 19 |
| Setup Java 21 | 0s | 0s | 0s | 1s | 19 |
| Test monitor-bot | 30s | 30s | 26s | 36s | 13 |
