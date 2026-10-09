# Tiscord

Go로 구현한 단일 화면 Vim 스타일 터미널 Discord 채팅 클라이언트.
서버 → 채널 → 채팅 순으로 이동하며 입력줄 아래에 모드·서버·채널·연결 상태를 표시합니다.

## 실행

빌드된 실행 파일은 Python, Node.js, Go 설치 없이 실행할 수 있습니다.
macOS, Linux, Windows를 대상으로 합니다. 실제 실행 검증은 Windows에서 수행하며,
macOS·Linux는 교차 빌드와 공통 동작 테스트로 검증합니다.

Windows PowerShell:

```powershell
.\dist\tiscord-windows-amd64.exe --demo
.\dist\tiscord-windows-amd64.exe
```

macOS Apple Silicon:

```sh
chmod +x dist/tiscord-darwin-arm64
./dist/tiscord-darwin-arm64 --demo
./dist/tiscord-darwin-arm64
```

Linux x86-64:

```sh
chmod +x dist/tiscord-linux-amd64
./dist/tiscord-linux-amd64 --demo
./dist/tiscord-linux-amd64
```

`--demo`는 토큰이나 네트워크 연결 없이 화면과 키 조작을 확인하는 오프라인 모드입니다.
처음 연결할 때 개인 계정 토큰을 숨김 입력으로 받고, 사용자 설정 폴더의 `tiscord/token` 파일에 저장합니다.
입력 직전에 프로그램이 웹 Discord에서 개발자 도구의 Network → Headers → authorization 값을 찾는 순서를 안내합니다.
다음 실행부터는 저장된 토큰을 자동으로 읽습니다. 토큰 파일은 평문이며 로그에는 출력하지 않습니다.

- Windows: `%AppData%\tiscord\token`
- macOS: `~/Library/Application Support/tiscord/token`
- Linux: `$XDG_CONFIG_HOME/tiscord/token`, 미설정 시 `~/.config/tiscord/token`

macOS·Linux에서는 새 설정 폴더를 `0700`, 토큰 파일을 `0600` 권한으로 만듭니다.
Windows에서는 사용자 설정 폴더의 접근 권한을 상속합니다.
토큰을 바꾸려면 파일을 수정하거나 삭제하고 다시 실행하세요.
`DISCORD_TOKEN` 환경변수는 저장된 토큰보다 우선하며, 환경변수의 값은 파일을 덮어쓰지 않습니다.

소스에서 실행하려면 Go 1.26 이상이 필요합니다:

```sh
go run . --demo
go build -o tiscord .
```

## 설정 파일

처음 실행할 때 토큰·로그와 같은 폴더에 `config.json`을 생성합니다.
Windows에서는 `%AppData%\tiscord\config.json`, macOS에서는 `~/Library/Application Support/tiscord/config.json`,
Linux에서는 `$XDG_CONFIG_HOME/tiscord/config.json` 또는 `~/.config/tiscord/config.json`입니다.
파일을 편집한 뒤 앱을 재실행하면 적용됩니다. 생략한 항목은 기본값을 사용합니다.
전체 기본값은 저장소의 `config.example.json`에서 확인할 수 있습니다.

예를 들어:

```json
{
  "behavior": {
    "group_messages": true,
    "group_window_seconds": 180,
    "send_key": "ctrl+s",
    "start_mode": "insert"
  },
  "layout": {
    "group_spacing": 0,
    "show_timestamps": true,
    "timestamp_format": "HH:mm",
    "input_max_lines": 3,
    "input_gap_lines": 0,
    "show_metadata": true
  },
  "logging": {
    "level": "debug"
  }
}
```

| 항목 | 기본값 | 허용 값 / 의미 |
| --- | --- | --- |
| `behavior.group_messages` | `true` | 같은 사용자 메시지 묶기 |
| `behavior.group_window_seconds` | `300` | 묶음 시간, 1–86400초 |
| `behavior.send_key` | `ctrl+s` | `ctrl+s` 또는 `enter`; Ctrl-S는 항상 전송 가능 |
| `behavior.start_mode` | `normal` | 채팅 화면 진입 시 `normal` 또는 `insert` |
| `layout.group_spacing` | `0` | 메시지 묶음 사이 빈 줄, 0–3 |
| `layout.show_timestamps` | `true` | 시간 표시 |
| `layout.timestamp_format` | `HH:mm` | `HH:mm`, `HH:mm:ss`, `YYYY-MM-DD HH:mm` |
| `layout.input_max_lines` | `3` | 입력창 최대 높이, 1–10줄 |
| `layout.input_gap_lines` | `1` | 채팅과 입력창 사이 빈 줄, 0–3 |
| `layout.show_metadata` | `true` | 하단 모드·채널·상태 표시 |
| `logging.level` | `debug` | `debug`, `info`, `warn`, `error`, `off` |

`send_key`를 `enter`로 정하면 Insert 모드에서 Enter로 전송하고 Ctrl-J로 줄바꿈합니다.
한글 조합 확정에 Enter를 사용하는 경우 기본 `ctrl+s` 설정이 편합니다.
JSON 문법·오타·범위 오류는 실행 시 표시하며 기존 설정 파일은 덮어쓰지 않습니다.
다른 설정 파일을 사용하려면 `--config 경로`를 지정하세요:

```powershell
.\dist\tiscord-windows-amd64.exe --demo --config .\config.example.json
```

## 조작

| 화면 / 모드 | 키 | 동작 |
| --- | --- | --- |
| 서버·채널 목록 | `j/k`, 방향키 | 이동 |
| 서버·채널 목록 | `/검색어` + Enter | 이름 필터, 빈 검색어로 해제 |
| 서버·채널 목록 | Enter | 선택 |
| 채팅 Normal | `j/k`, `gg/G` | 메시지 스크롤, 처음/최신 이동 |
| 채팅 Normal | `i/a`, `I/A` | 입력 모드, 커서 뒤/줄 시작/줄 끝에서 입력 |
| 채팅 Normal | `h/l`, `w/b`, `0/$` | 입력 커서 이동 |
| 채팅 Normal | `x`, `dd` | 입력 문자/줄 삭제 |
| 채팅 Insert | Enter | 줄바꿈 |
| 채팅 | Ctrl-S | 전송 |
| 채팅 Normal | Ctrl-U 또는 `:older` | 이전 메시지 조회 |
| Insert / 명령 입력 | Esc | Normal로 복귀 |
| Normal | `:channels`, `:servers` | 탐색 화면으로 이동 |
| Normal | `:send`, `:?` 또는 `:help`, `:q` | 전송, 명령어 목록과 설명, 종료 |
| 명령 입력 | Tab / Shift-Tab | 입력한 접두어와 일치하는 명령을 다음 / 이전 후보로 순환 |
| 전체 | Ctrl-C / Ctrl-Q | 종료 |

Enter는 전송하지 않으므로 한글 조합 확정 중 메시지가 전송되지 않습니다.
`:?` 도움말 화면은 Esc 또는 Enter로 닫고, `j/k`로 스크롤합니다. 작성 중인 메시지는 유지됩니다.
`:`만 입력하고 Tab을 누르면 전체 명령을 순환하며, `:s`에서는 `:servers`와 `:send`만 순환합니다.
채널별 초안은 실행 중 메모리에 보존하며 종료하면 사라집니다.
같은 사용자 ID의 연속 메시지는 직전 메시지와 5분 이내이면 시간·이름을 한 번만 표시하고 내용을 이어 붙입니다.
다른 사용자가 끼어들거나 시간이 더 벌어지면 새 묶음을 시작합니다. `j/k`는 묶음 안에서도 메시지 단위로 이동합니다.
Normal/Insert와 위 표의 Vim 키를 지원합니다. 전체 Vim 명령과 undo는 아직 지원하지 않습니다.

## 메시지 수신

- 새 메시지는 Discord Gateway WebSocket의 `MESSAGE_CREATE` 이벤트로 수신합니다. 주기적인 메시지 조회는 하지 않습니다.
- 수정, 삭제, 일괄 삭제 이벤트도 현재 채널에 반영합니다. 다른 채널의 메시지는 표시하지 않습니다.
- heartbeat와 ACK로 연결 상태를 확인합니다. heartbeat는 채팅 조회 요청이 아닙니다.
- 연결이 끊기면 마지막 이벤트 시퀀스와 세션으로 Resume을 시도합니다.
- Resume이 불가능해 새 세션으로 접속하면 현재 채널의 최근 기록을 한 번 다시 조회합니다.
- 최초 채널 기록, 이전 메시지, 전송은 REST HTTP를 사용합니다.
- 개인 계정용 서버 이벤트 구독을 Gateway에 전송합니다. 현재 탐색 중인 서버를 구독하며 전체 서버의 메시지를 수집하지 않습니다.
- 기록을 읽는 동안 도착한 이벤트는 기록 로드 후 다시 적용하고, 전송 응답과 수신 이벤트의 중복은 메시지 ID로 제거합니다.
- 요청 제한 시간을 따릅니다. 전송의 성공 여부가 불확실한 네트워크 오류는 자동 재전송하지 않고 초안을 유지합니다.

텍스트·공지 채널 탐색과 채팅만 지원합니다. 첨부파일·임베드는 URL 또는 텍스트로 표시합니다.
DM, 스레드, 음성, 리액션, 읽음 처리는 초기 범위에서 제외합니다.

개인 계정 인증과 이벤트 구독은 공식 지원 API가 아니며 Discord 변경으로 동작하지 않을 수 있습니다.
실제 개인 계정 연결은 아직 검증하지 않았습니다. Discord는 OAuth2/bot API 밖의 일반 계정 자동화를 금지합니다:
[Discord 공식 안내](https://support.discord.com/hc/en-us/articles/115002192352-Automated-User-Accounts-Self-Bots).

프로토콜 참고:
[Discord Gateway](https://docs.discord.com/developers/events/gateway),
[개인 계정 Guild Subscriptions](https://discordpy-self.readthedocs.io/en/latest/guild_subscriptions.html).
UI는 Bubble Tea, WebSocket은 Gorilla WebSocket을 사용합니다.

## 검증

로그는 사용자 설정 폴더의 `tiscord/tiscord.log`에 JSON Lines 형식으로 추가 기록합니다. 기본 로그 수준은 `debug`이며 설정으로 변경할 수 있습니다.
실행 시 로그 경로를 표시합니다. HTTP 요청 경로·상태·응답 시간·제한 처리, Gateway opcode·이벤트·연결·heartbeat·재연결,
화면 전환과 기록 로드·전송 결과를 남깁니다. 토큰, 인증 헤더, Gateway 원문, 메시지 본문과 입력 키 값은 기록하지 않습니다.

Windows에서 실시간으로 로그 보기:

```powershell
Get-Content "$env:APPDATA\tiscord\tiscord.log" -Tail 100 -Wait
```

macOS·Linux에서는 위의 토큰 파일과 같은 디렉터리의 `tiscord.log`를 `tail -f`로 확인할 수 있습니다.

```sh
go test ./...
go vet ./...
```

모의 HTTP 응답, 로컬 WebSocket 서버, 오프라인 키 이벤트로 검증합니다.
테스트는 Discord에 연결하거나 실제 계정으로 메시지를 보내지 않습니다.
