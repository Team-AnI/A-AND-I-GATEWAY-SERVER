# CI/CD Metrics Collector

Gateway 이력서 수치화를 위해 GitHub Actions run history를 읽기 전용으로 수집합니다.

## What It Collects

- CI 전체 시간
- `./gradlew test`
- `cd monitor-bot && go test ./...`
- `./gradlew build` 또는 `./gradlew build -x test`
- k6 install
- performance asset validation
- CD의 Gateway Docker build/push
- CD의 monitor-bot Docker build/push
- CD의 deploy step duration metadata

각 항목은 최근 N회 기준 평균, 중앙값, 최소, 최대, 실패율을 계산합니다.

3회 미만인 수치는 이력서 사용 비추천으로 표시합니다.

## Safety

이 스크립트는 다음 GitHub CLI 조회만 실행합니다.

```bash
gh run list
gh run view --json jobs
```

다음 작업은 수행하지 않습니다.

- `workflow_dispatch`
- tag push
- AWS CLI 실행
- SSH 접속
- docker push
- 운영 URL 접근
- secrets, AWS host, EC2 host, SSH key, token 값 출력 또는 저장

## Usage

```bash
python3 scripts/ci/collect_github_actions_metrics.py \
  --repo Team-AnI/A-AND-I-GATEWAY-SERVER \
  --workflow "CI" \
  --branch main \
  --limit 20 \
  --out-json docs/metrics/gateway-ci-summary.json \
  --out-md docs/ci-metrics.md
```

CD workflow는 기본값 `CD`로 함께 조회합니다.

CD를 제외해야 할 때만 `--skip-cd`를 사용합니다.

```bash
python3 scripts/ci/collect_github_actions_metrics.py \
  --repo Team-AnI/A-AND-I-GATEWAY-SERVER \
  --workflow "CI" \
  --branch main \
  --limit 20 \
  --skip-cd \
  --out-json docs/metrics/gateway-ci-summary.json \
  --out-md docs/ci-metrics.md
```

## Outputs

- `docs/metrics/gateway-ci-summary.json`: 실제 측정 결과
- `docs/ci-metrics.md`: 리뷰 가능한 Markdown 표
- `docs/metrics/gateway-ci-summary.example.json`: 스키마 예시
