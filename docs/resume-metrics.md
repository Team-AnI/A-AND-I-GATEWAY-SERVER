# Gateway Resume Metrics

| 영역 | 수치 | 신뢰도 | 근거 파일 | 이력서 문장 | 사용 여부 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| CI 전체 시간 | Before median 2m 28s, After median 3m 23s, 개선율 -37.6% | 확인 완료 | [docs/cicd-optimization.md](./cicd-optimization.md), [docs/metrics/gateway-cicd-before-after.json](./metrics/gateway-cicd-before-after.json) | [사용 비추천] GitHub Actions CI 전체 시간 단축은 before/after median 기준으로 확인되지 않음 | 사용 비추천 |
| Monitor Bot test | Before median 30s, After median 11s, 개선율 63.3% | 확인 완료 | [docs/cicd-optimization.md](./cicd-optimization.md), [docs/metrics/gateway-cicd-before-after.json](./metrics/gateway-cicd-before-after.json) | GitHub Actions에서 Monitor Bot test를 별도 job으로 분리해 median 30s -> 11s로 63.3% 단축 | 사용 가능 |
| CD dry-run | CD before metadata median 4m 18s, CD Dry Run median 2m 29s, 운영 배포 실행 false | 확인 완료 | [docs/cicd-optimization.md](./cicd-optimization.md), [docs/metrics/gateway-cicd-before-after.json](./metrics/gateway-cicd-before-after.json) | [사용 비추천] 기존 CD는 운영 build/push/deploy metadata이고 after는 push:false dry-run이므로 운영 배포 시간 단축으로 쓰지 않음 | 사용 비추천 |
| Gateway image dry-run build | 기존 CD Gateway image build/push step median 25s, dry-run image build job median 19s | 확인 완료 | [docs/cicd-optimization.md](./cicd-optimization.md), [docs/metrics/gateway-cicd-before-after.json](./metrics/gateway-cicd-before-after.json) | [사용 비추천] before step은 push 포함, after step은 push:false dry-run이므로 image build 개선으로 직접 사용하지 않음 | 사용 비추천 |
