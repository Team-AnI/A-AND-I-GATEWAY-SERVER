# Gateway CI/CD Optimization

> GitHub Actions metadata 기준입니다. 운영 배포 시간이나 운영 트래픽 처리량이 아닙니다.

## Before/After

| 항목 | Before median | After median | 개선율 | 신뢰도 | 사용 여부 |
| :--- | ---: | ---: | ---: | :--- | :--- |
| CI 전체 시간 | 2m 28s | 3m 23s | -37.6% | 확인 완료 | 사용 비추천 |
| Backend test | 1m 42s | 1m 54s | -12.3% | 확인 완료 | 사용 비추천 |
| Monitor Bot test | 30s | 11s | 63.3% | 확인 완료 | 사용 가능 |
| Performance asset validation | 2s | 7s | -250.0% | 확인 완료 | 사용 비추천 |
| Build JAR | 2s | 1m 14s | -3600.0% | 확인 완료 | 사용 비추천 |
| Gateway image dry-run build | 25s | 19s | 24.0% | 확인 완료 | 사용 비추천 |
| Monitor Bot image dry-run build | 38s | 44s | -15.8% | 확인 완료 | 사용 비추천 |
| CD dry-run 전체 시간 | 4m 18s | 2m 29s | 42.2% | 확인 완료 | 사용 비추천 |

## Safety

- Production deploy executed: false
- AWS/ECR/SSH executed: false
- Docker push executed: false
- GitHub Actions metadata read-only 조회

## Resume Sentence Candidates

[사용 비추천] GitHub Actions CI 전체 시간 단축은 before/after median 기준으로 확인되지 않음
GitHub Actions에서 Monitor Bot test를 별도 job으로 분리해 median 30s → 11s로 63.3% 단축
[측정 필요] CD dry-run image build before/after median 표본이 부족하거나 직접 비교가 어려워 이력서 사용 비추천
