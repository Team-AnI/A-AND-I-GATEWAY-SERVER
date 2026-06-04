# Demo Capture

> 랜딩 페이지로 돌아가기: [README](../README.md)

본 문서는 A-AND-I-GATEWAY-SERVER의 데모 asset과 수동 촬영 기준을 기록합니다. 운영 Discord token, CloudWatch Logs 접근권, guild/role 설정이 필요한 흐름은 실제 운영 값을 호출하지 않습니다.

## 생성된 데모 파일

| 기능 | 파일 | 생성 방식 | 상태 |
| :--- | :--- | :--- | :--- |
| Architecture | `docs/assets/diagrams/architecture.png` | 다이어그램 PNG | 생성 |
| Operation flow | `docs/assets/diagrams/ops-flow.png` | 운영 알림/trace 흐름 다이어그램 | 생성 |
| Critical alert | `docs/assets/gifs/critical-alert.gif` | mock event / local fixture 기반 GIF | 생성 |
| Trace drilldown | `docs/assets/gifs/trace-drilldown.gif` | mock traceId / local fixture 기반 GIF | 생성 |
| Error code policy | `docs/assets/images/error-code-policy.png` | 공통 응답과 5자리 error code 구조 이미지 | 생성 |
| Coverage | `docs/assets/images/coverage-report.png` | JaCoCo HTML report screenshot | 생성 |

## Mock GIF 기준

`critical-alert.gif`와 `trace-drilldown.gif`는 실제 Discord와 CloudWatch를 호출하지 않습니다. 운영 token, guild ID, role ID, 실제 log group을 노출하지 않기 위해 mock event와 mock traceId만 사용했습니다.

표시한 흐름:

- critical severity log가 critical alert route로 분기됩니다.
- critical alert만 role mention 정책을 사용합니다.
- traceId로 CloudWatch Logs Insights 조회 흐름을 따라갑니다.
- request, policy check, error response 로그를 묶어 원인 후보를 확인합니다.

## 자동 생성에 사용한 명령

Coverage screenshot:

```bash
./gradlew jacocoTestReport
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --headless=new \
  --disable-gpu \
  --hide-scrollbars \
  --window-size=1400,900 \
  --screenshot=docs/assets/images/coverage-report.png \
  "file://$PWD/build/reports/jacoco/test/html/index.html"
```

Gateway 로컬 smoke check는 내부 이벤트 검증값을 로컬 더미값으로 주입해 실행했습니다. 더미값 자체는 문서에 남기지 않습니다.

```bash
SERVER_PORT=18080 MANAGEMENT_SERVER_PORT=19090 ./gradlew bootRun
curl -sS -i http://localhost:19090/actuator/health
curl -sS -i http://localhost:18080/not-allowlisted
```

## 실제 Discord / CloudWatch 촬영 기준

실제 staging 환경에서 GIF를 다시 촬영하려면 아래 조건이 필요합니다.

| 기능 | 권장 파일명 | 필요한 환경 | 촬영 범위 |
| :--- | :--- | :--- | :--- |
| Critical alert | `docs/assets/gifs/critical-alert.gif` | 테스트 Discord guild, staging role, staging log group | general/critical channel 분리, critical에서만 role mention 표시 |
| Trace drilldown | `docs/assets/gifs/trace-drilldown.gif` | staging traceId, CloudWatch Logs Insights 접근권 | traceId 입력, 관련 로그 조회, 원인 후보 확인 |

## 민감정보 제거 기준

- Discord token, public key, application ID, guild ID, role ID 원문을 노출하지 않습니다.
- Authorization, Authenticate, refreshToken, accessToken, password, salt, secret을 노출하지 않습니다.
- 실제 사용자명, 이메일, 운영 서버 주소는 mock 값으로 대체합니다.
- CloudWatch raw log의 request body와 full response data는 표시하지 않습니다.
