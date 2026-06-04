# Critical Alert Routing Troubleshooting

> 문서 목차로 돌아가기: [Gateway Docs](../README.md)

## 증상

- critical alert가 general channel로만 전송됩니다.
- role mention이 표시되지 않습니다.
- HIGH alert에서 role mention이 기대와 다르게 표시되지 않습니다.
- 반복 alert가 계속 오거나 반대로 alert가 suppressed 됩니다.

## 확인 순서

1. `/ops alert action:status`로 general channel, critical channel, role mention, cooldown을 확인합니다.
2. critical channel이 없으면 `/ops alert action:channel target:critical channel:#ops-critical`로 설정합니다.
3. general channel이 없으면 `/ops alert action:channel target:general channel:#ops-log`로 설정합니다.
4. role mention은 `/ops alert action:role role:@운영팀`으로 설정하고, bot이 해당 role을 mention할 권한이 있는지 확인합니다.
5. alert가 `CRITICAL`인지, 아니면 `HIGH`인지 structured log의 `level`, `response.error.code`, `logType`을 확인합니다.
6. 같은 incident가 cooldown 내에 반복된 경우 state suppression으로 전송되지 않을 수 있습니다.

## 정책

| alert | route | role mention |
| :--- | :--- | :--- |
| CRITICAL | critical | configured role 허용 |
| P0 legacy alert | critical | configured role 허용 |
| HIGH service alert | general | 없음 |
| assignment audit | general | 없음 |
| assignment WARN/INFO | general | 없음 |

`@everyone`, `@here`는 허용하지 않습니다.

## 코드 근거

- alert route와 role validation: `monitor-bot/internal/monitor/alerts.go`
- severity decision: `monitor-bot/internal/opslog/v2.go`
- alert query: `monitor-bot/internal/cloudwatch/queries.go`
- state store: `monitor-bot/internal/state/store.go`

## 점검 포인트

- `response.error.code`가 5자리 정책에 맞지 않으면 service/category 판단이 `UNKNOWN_ERROR_CODE`로 표시될 수 있습니다.
- critical route fallback은 state critical channel -> legacy alert channel -> env alert channel 순서입니다.
- general route fallback은 state general channel -> legacy alert channel -> env alert channel -> dashboard channel 순서입니다.
