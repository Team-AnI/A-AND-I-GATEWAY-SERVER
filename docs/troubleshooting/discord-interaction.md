# Discord Interaction Troubleshooting

> 문서 목차로 돌아가기: [Gateway Docs](../README.md)

## 증상

- `/ops` command가 Discord에 보이지 않습니다.
- command 실행 시 권한 없음 메시지가 표시됩니다.
- interaction 요청이 401 또는 405로 실패합니다.
- 버튼 클릭 후 fallback 안내만 표시됩니다.

## 확인 순서

1. `/healthz`에서 `discordCommandsRegistered`와 `discordCommandRegistrationError`를 확인합니다.
2. command schema를 바꾼 배포라면 등록 시점에만 `DISCORD_REGISTER_COMMANDS=true`로 실행했는지 확인합니다.
3. nginx가 `/discord/interactions`를 monitor-bot `http://monitor-bot:8088/interactions`로 프록시하는지 확인합니다.
4. Discord request signature header가 전달되는지 확인합니다.
5. `DISCORD_ALLOWED_GUILD_ID`와 `DISCORD_ALLOWED_ROLE_IDS`가 테스트 guild/role과 일치하는지 확인합니다.

## 코드 근거

- Signature 검증: `monitor-bot/internal/discord/signature.go`
- Interaction handler: `monitor-bot/internal/discord/interactions.go`
- Command registration: `monitor-bot/internal/discord/commands.go`
- Health endpoint: `monitor-bot/internal/health/server.go`
- nginx 프록시: `.github/workflows/cd.yml`

## 주의

- Discord token, public key, application ID, guild ID 원문을 공유 채널이나 screenshot에 노출하지 않습니다.
- registration failure는 `STRICT_STARTUP_CHECKS=true`가 아니면 process restart로 이어지지 않고 `/healthz`에 남습니다.
- 버튼 interaction은 ephemeral follow-up을 기본으로 사용하므로 public channel에 로그 결과가 보이지 않을 수 있습니다.
