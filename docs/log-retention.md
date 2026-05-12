# Log Retention Policy

## 원칙

로그 보관/정리는 애플리케이션 내부 Quartz Cron으로 처리하지 않는다. Gateway Spring Boot 앱과 monitor-bot은 CloudWatch 로그를 삭제하지 않으며, Discord 명령도 삭제 기능을 제공하지 않는다.

운영 로그 관리는 인프라 레벨에서 나눈다.

- CloudWatch Logs: retention policy
- EC2 local Docker logs: Docker log rotation
- Docker image/cache: Amazon Linux 2023 systemd timer

## CloudWatch Logs Retention

CloudWatch Logs는 retention이 없으면 무기한 보관될 수 있다. CD는 기본 14일 retention을 다음 log group에 적용한다.

- `/a-and-i/gateway`
- `/a-and-i/prod/monitor-bot`
- `/a-and-i/prod/report`
- `/a-and-i/prod/report-mongodb`
- `/a-and-i/auth`
- `/a-and-i/online-judge`
- `/a-and-i/prod/tech-blog`

GitHub Variable `CLOUDWATCH_LOG_RETENTION_DAYS`로 변경할 수 있고, 값이 없으면 14일을 사용한다. 장애 조사 기간이 부족하면 30일로 늘릴 수 있다.

Retention 변경 후 기존 로그 삭제가 즉시 반영되지 않을 수 있다. CloudWatch Logs 보관 정책은 원격 로그 보관 기간만 제어하며, EC2 로컬 디스크 정리와는 별개다.

## Required IAM

Retention 설정 권한:

```json
{
  "Effect": "Allow",
  "Action": [
    "logs:CreateLogGroup",
    "logs:PutRetentionPolicy",
    "logs:DescribeLogGroups"
  ],
  "Resource": "*"
}
```

monitor-bot 조회 권한:

```json
{
  "Effect": "Allow",
  "Action": [
    "logs:StartQuery",
    "logs:GetQueryResults",
    "logs:DescribeLogGroups",
    "logs:DescribeLogStreams",
    "cloudwatch:DescribeAlarms"
  ],
  "Resource": "*"
}
```

로그 쓰기 권한:

```json
{
  "Effect": "Allow",
  "Action": [
    "logs:CreateLogGroup",
    "logs:CreateLogStream",
    "logs:PutLogEvents",
    "logs:DescribeLogStreams"
  ],
  "Resource": "*"
}
```

## Docker Local Logs

`awslogs` driver를 쓰는 서비스는 CloudWatch retention으로 원격 로그 보관을 제어한다. `json-file` driver를 쓰는 서비스는 로컬 디스크 증가를 막기 위해 rotation을 둔다.

```yaml
logging:
  driver: json-file
  options:
    max-size: "20m"
    max-file: "3"
```

`awslogs` driver에는 `max-size`, `max-file`을 섞지 않는다.

## Discord 조회

monitor-bot의 `/disk`, `/retention` 명령은 CloudWatch `DescribeLogGroups` 결과의 `storedBytes`, `retentionInDays`만 보여준다.

- host Docker 로그를 직접 조회하지 않음
- Docker socket을 마운트하지 않음
- 로그 삭제 또는 prune 명령을 실행하지 않음
- retention이 없으면 `INFINITE`로 표시
