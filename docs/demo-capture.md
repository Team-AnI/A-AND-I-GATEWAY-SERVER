# Demo Capture

> 메인 README로 돌아가기: [README](../README.md)

본 프로젝트는 backend/Gateway와 Discord 운영 bot 중심이라 실제 Discord GIF 자동 생성에는 운영 Discord token, CloudWatch Logs 접근권, 민감정보가 제거된 테스트 데이터가 필요합니다. 이번 작업에서는 민감정보 없는 로컬 Gateway 응답 screenshot만 생성하고, Discord/CloudWatch 흐름은 실패 원인과 수동 촬영 기준을 남깁니다.

## 생성된 데모 파일

| 기능 | 파일 | 생성 방식 | 상태 |
| :--- | :--- | :--- | :--- |
| Gateway routing | `docs/assets/gifs/gateway-routing-demo.gif` | 수동 촬영 필요 | 미생성 |
| Discord dashboard | `docs/assets/gifs/discord-dashboard-demo.gif` | 수동 촬영 필요 | 미생성 |
| Critical alert | `docs/assets/gifs/critical-alert-demo.gif` | 수동 촬영 필요 | 미생성 |
| Trace drilldown | `docs/assets/images/trace-drilldown-example.png` | 수동 촬영 필요 | 미생성 |
| Error code policy | `docs/assets/images/error-code-policy.png` | Chrome headless screenshot | 생성 |

## 자동 생성 시도 결과

| 항목 | 결과 |
| :--- | :--- |
| 로컬 실행 | 성공 |
| 브라우저 실행 | Chrome headless screenshot 성공 |
| GIF 생성 | 실패로 기록 |
| 생성 파일 | `docs/assets/images/error-code-policy.png` |
| 실패 원인 | Discord Interactions는 `DISCORD_PUBLIC_KEY`, `DISCORD_BOT_TOKEN`, guild/role 설정과 CloudWatch Logs 접근권이 필요합니다. 민감정보 노출 위험이 있어 실제 운영 Discord/CloudWatch를 자동 호출하지 않았습니다. |

## 자동 생성에 사용한 명령

```bash
INTERNAL_EVENT_TOKEN=local-test-token SERVER_PORT=18080 MANAGEMENT_SERVER_PORT=19090 ./gradlew bootRun
curl -sS -i http://localhost:19090/actuator/health
curl -sS -i http://localhost:18080/not-allowlisted
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new --disable-gpu --screenshot=docs/assets/images/error-code-policy.png --window-size=1000,700 http://localhost:18080/not-allowlisted
```

## 수동 촬영 기준

| 기능 | 권장 파일명 | 촬영 범위 | 반드시 보여줄 액션 |
| :--- | :--- | :--- | :--- |
| Gateway routing | `docs/assets/gifs/gateway-routing-demo.gif` | 민감정보 없는 local/staging Gateway 요청 | allowlisted route 성공, denylisted route `15001` 실패 응답 |
| Discord dashboard | `docs/assets/gifs/discord-dashboard-demo.gif` | 테스트 Discord guild의 `/ops dashboard` | service summary, 최근 alert/incident grouping |
| Critical alert | `docs/assets/gifs/critical-alert-demo.gif` | 테스트 channel의 `/ops alert`와 critical test log | general/critical channel 분리, role mention은 critical에서만 표시 |
| Trace drilldown | `docs/assets/images/trace-drilldown-example.png` | alert 메시지와 `/ops logs mode:trace` 결과 | traceId, requestId, HTTP status, error code |
| Assignment audit | `docs/assets/gifs/assignment-audit-demo.gif` | 테스트 Report EVENT log와 `/ops logs mode:events` | actor, occurredAt, assignmentId, changedFields, source |
| Error code policy | `docs/assets/images/error-code-policy.png` | curl 또는 API client 응답 | `success=false`, `error.code`, `value`, `alert`, `timestamp` |

## 민감정보 제거 기준

- Discord token, public key, application ID, guild ID, role ID 원문을 노출하지 않습니다.
- Authorization, Authenticate, refreshToken, accessToken, password, salt, secret을 노출하지 않습니다.
- 실제 사용자명, 이메일, 운영 서버 주소는 mock 값으로 대체합니다.
- CloudWatch raw log의 request body와 full response data는 표시하지 않습니다.
