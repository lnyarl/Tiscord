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

## 바이너리 릴리즈

GitHub Actions는 `v0.1.0` 같은 버전 태그를 푸시하면 테스트와 `go vet`를 실행하고 GitHub Release를 발행합니다. Windows x86-64는 ZIP, macOS Intel/Apple Silicon과 Linux x86-64/ARM64는 tar.gz로 제공하며, SHA-256 체크섬도 첨부합니다. 각 압축 파일에는 실행 파일, README, 예제 설정만 포함됩니다. 토큰·로그·사용자 설정은 포함하지 않습니다.

워크플로 파일을 먼저 커밋·푸시한 뒤 릴리즈할 커밋에 태그를 붙입니다:

```sh
git tag v0.1.0
git push origin v0.1.0
```

`v0.1.0-rc.1`처럼 하이픈이 있는 태그는 사전 릴리즈로 표시됩니다. 업로드가 완료되기 전에는 draft 상태로 유지합니다. 릴리즈 파일은 [GitHub Releases](https://github.com/lnyarl/Tiscord/releases)에서 내려받아 압축을 풀고 `tiscord.exe` 또는 `./tiscord`를 실행하세요. macOS·Linux에서 실행 권한이 없으면 `chmod +x tiscord`를 실행합니다. 코드 서명·공증은 포함하지 않습니다.

로컬에서 발행 없이 같은 패키지를 만들려면:

```powershell
./scripts/build-release.ps1 -Tag v0.1.0
```

결과는 `dist/release/v0.1.0/`에 생성됩니다. Go, PowerShell, `tar`가 필요합니다.

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
| `behavior.korean_shortcuts` | `true` | 물리 키 정보가 없는 Normal 입력의 두벌식 한글 단축키 해석 |
| `layout.group_spacing` | `0` | 메시지 묶음 사이 빈 줄, 0–3 |
| `layout.style` | `plain` | `plain`(기존), `log`(로그), `editor`(줄 번호·주석 형식) |
| `layout.show_timestamps` | `true` | 시간 표시 |
| `layout.timestamp_format` | `HH:mm` | `HH:mm`, `HH:mm:ss`, `YYYY-MM-DD HH:mm` |
| `layout.input_max_lines` | `3` | 입력창 최대 높이, 1–10줄 |
| `layout.input_gap_lines` | `1` | 채팅과 입력창 사이 빈 줄, 0–3 |
| `layout.show_metadata` | `true` | 하단 모드·채널·상태 표시 |
| `layout.show_typing` | `true` | 입력창 아래에 현재 채널의 입력 중인 사람 표시 |
| `layout.author_name` | `"username"` | 채팅과 입력 중 표시에 계정 username 사용. `"display"`는 표시 이름 사용 |
| `behavior.restore_unread` | `true` | 채널 진입 시 Discord의 마지막 읽음 위치 다음 메시지로 이동 |
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
| 채널 목록 | Esc | 이전에 보던 채팅으로 복귀 (초안·스크롤 위치 보존) |
| 서버 목록 | Esc | 이전에 보던 채널 목록으로 복귀; 다시 Esc로 이전 채팅 복귀 |
| Normal | `:channels`, `:servers` | 탐색 화면으로 이동 |
| Normal | `:send`, `:?` 또는 `:help`, `:q` | 전송, 명령어 목록과 설명, 종료 |
| Normal | `:sync` | 연결된 현재 채널의 기록 다시 동기화 |
| Normal | `:layout` | plain → log → editor 순환 |
| Normal | `:layout plain`, `:layout log`, `:layout editor` | 형식 직접 선택 |
| 명령 입력 | Tab / Shift-Tab | 입력한 접두어와 일치하는 명령을 다음 / 이전 후보로 순환 |
| 전체 | Ctrl-C / Ctrl-Q | 종료 |

Enter는 전송하지 않으므로 한글 조합 확정 중 메시지가 전송되지 않습니다.
Normal 모드에서는 터미널이 제공하는 기본 키 위치 정보(`BaseCode`)를 우선 사용합니다.
따라서 한글 글자와 함께 `j/k/i` 키 정보가 전달되면 스크롤·입력 전환은 영문 키 기준으로 동작합니다.
Insert 모드에서는 IME가 만든 한글 텍스트를 그대로 입력합니다. 운영체제의 한영 상태 자체를 변경하지는 않습니다.
Windows 콘솔과 키보드 향상 프로토콜을 지원하는 터미널에서 키 정보를 받을 수 있으며,
물리 키 정보가 없는 환경에서는 기본적으로 두벌식 한글을 Normal 단축키로 해석합니다 (`ㅓ → j`, `ㅏ → k`, `ㅑ → i`, `ㅎㅎ → gg`).
한글 자모와 조합된 음절을 처리하며 Insert·검색·명령 입력과 붙여넣기는 변환하지 않습니다.
`behavior.korean_shortcuts: false`로 이 보조 처리를 끌 수 있습니다. IME가 글자를 확정할 때까지 전달을 지연하면 동작도 확정 후 발생할 수 있습니다.
터미널 프로토콜 지원 여부는 로그의 `keyboard.capabilities`에 기록합니다. 입력 문자와 키 값은 로그에 남기지 않습니다.
`:?` 도움말 화면은 Esc 또는 Enter로 닫고, `j/k`로 스크롤합니다. 작성 중인 메시지는 유지됩니다.
`:`만 입력하고 Tab을 누르면 전체 명령을 순환하며, `:s`에서는 `:servers`, `:send`, `:sync`를 순환합니다.
`:layout ` 뒤에서 Tab을 누르면 형식 후보를 순환합니다. 선택한 형식은 사용하는 설정 파일의 `layout.style`에 자동 저장되어 다음 실행에도 유지됩니다. `--config`로 지정한 파일에도 동일하게 저장합니다.

`:config`로 앱 안에서 설정을 수정할 수 있습니다. `j/k` 또는 위/아래 방향키로 항목을 선택하고, `h/l` 또는 좌/우 방향키로 값을 변경합니다. `Enter`나 Space도 값을 바꿉니다. 숫자는 한 번에 1씩 변경합니다. `Ctrl-S`는 설정 파일에 저장하고 즉시 적용하며, `Esc`는 저장하지 않고 돌아갑니다. 채널 진입 모드는 다음 채널 진입부터 적용됩니다. 설정 화면을 열어도 채팅 초안과 선택 위치는 유지됩니다.

채팅 화면의 Normal 모드에서 `/` 또는 `:slash`를 입력하면 Discord 봇의 슬래시 명령 입력 도우미가 열립니다. Insert 모드의 빈 입력창에서도 `/`로 열 수 있습니다. 서버와 사용자 앱의 명령 목록을 합쳐 표시하며, 문자 입력으로 검색하고 ↑↓/Tab으로 후보를 이동한 뒤 Enter로 선택합니다. 기존 채팅 초안과 메시지 선택 위치는 유지됩니다.

인자는 순서대로 입력하고 Enter로 확정합니다. 필수 여부·자료형·설명을 입력창 아래에 표시하며, 선택 인자는 빈 값 Enter로 건너뛸 수 있습니다. 고정 선택지와 봇의 동적 자동완성 결과는 Tab으로 입력합니다. 동적 자동완성은 300ms 입력 대기 후 요청하고, 이전 입력에 대한 늦은 결과는 무시합니다. 사용자·채널·역할 인자는 숫자 ID 또는 `<@ID>`, `<#ID>`, `<@&ID>` 형식으로 입력합니다. 첨부파일 인자는 아직 지원하지 않습니다.

모든 인자를 입력한 뒤 확인 화면에서 Ctrl-S로 실행합니다. 명령 실행은 일반 채팅 전송과 별도의 interaction 요청이며, Gateway 세션과 연결 복구가 필요합니다. 서버 권한 검증은 Discord가 수행합니다. 실행 중 중복 요청을 막고, 응답이 불확실하거나 시간 초과된 요청은 자동 재전송하지 않습니다. Discord에서 결과를 확인한 뒤 수동으로 재시도하세요. Esc는 이전 인자로 돌아가거나 입력 도우미를 닫습니다. `/`로 시작하는 텍스트를 일반 메시지로 보내려면 `//`로 시작하도록 붙여넣으세요. 전송할 때 첫 `/` 하나를 제거합니다.

명령 목록 조회·실행·자동완성은 공식 웹 클라이언트에서 확인한 개인 계정용 내부 경로를 사용하므로 향후 변경될 수 있습니다. 모달·버튼 조작과 비공개(ephemeral) 응답 전용 표시는 아직 지원하지 않습니다. `--demo`에서는 네트워크 없이 `/demo` 명령으로 입력 흐름을 확인할 수 있습니다. 실제 명령 목록 조회는 확인했으며, 실제 봇 명령 실행과 동적 자동완성은 로컬 HTTP/Gateway 테스트로 검증했습니다.
채널별 초안은 실행 중 메모리에 보존하며 종료하면 사라집니다.
같은 사용자 ID의 연속 메시지는 직전 메시지와 5분 이내이면 시간·이름을 한 번만 표시하고 내용을 이어 붙입니다.

봇의 임베드 응답은 제목·설명·필드·푸터를 일반 텍스트로 표시합니다. 첨부파일과 임베드 이미지·영상은 URL로 표시하며, 버튼·선택 메뉴 조작은 지원하지 않습니다.

채널에 들어가면 Discord에서 받은 읽음 위치를 기준으로 첫 번째 안 읽은 메시지부터 표시하고 `여기부터 새 메시지` 구분선을 넣습니다. 위치가 최신 50개보다 오래되면 그 지점까지 이전 페이지를 불러옵니다. 읽음 기록이 없는 채널은 최신 메시지로 시작합니다. 새 메시지가 와도 읽던 메시지가 화면에서 밀려나지 않도록 위치를 유지합니다.

Normal 모드의 `G`, `j`/↓ 또는 마우스 휠 아래 이동으로 최신 메시지에 도달하면 그 순간의 최신 메시지까지 Discord에 읽음 위치를 저장합니다. 채널 진입·화면 표시·새 메시지 도착만으로는 읽음 처리하지 않습니다. 저장 중 새 메시지가 오면 그 메시지는 안 읽은 상태로 남습니다. 다른 Discord 클라이언트의 읽음 변경도 Gateway를 통해 반영하며, 연결 복구 중에는 저장하지 않습니다. `:config`의 첫 안 읽은 메시지로 진입 항목으로 진입 위치 복원을 끌 수 있습니다.
다른 사용자가 끼어들거나 시간이 더 벌어지면 새 묶음을 시작합니다. `j/k`는 묶음 안에서도 메시지 단위로 이동합니다.
Normal/Insert와 위 표의 Vim 키를 지원합니다. 전체 Vim 명령과 undo는 아직 지원하지 않습니다.

## 메시지 수신

- 새 메시지는 Discord Gateway WebSocket의 `MESSAGE_CREATE` 이벤트로 수신합니다. 주기적인 메시지 조회는 하지 않습니다.
- 수정, 삭제, 일괄 삭제 이벤트도 현재 채널에 반영합니다. 다른 채널의 메시지는 표시하지 않습니다.
- `TYPING_START` 이벤트로 현재 채널의 입력 중인 사람을 입력창 아래에 표시합니다. 본인을 제외하고, 메시지를 보내거나 마지막 입력 이벤트 후 10초가 지나면 해제합니다.
- 입력 중인 사람이 여러 명이면 이름 두 개와 나머지 인원수를 표시합니다. 채널 이동·연결 종료 시 상태를 정리하며, 추가 HTTP 조회는 하지 않습니다.
- heartbeat와 ACK로 연결 상태를 확인합니다. heartbeat는 채팅 조회 요청이 아닙니다.
- 연결이 끊기면 마지막 이벤트 시퀀스와 세션으로 Resume을 시도합니다.
- 연결이 끊겼거나 메시지를 복구 중이면 전송을 막고 초안을 유지합니다. 전송 불가 안내는 메타정보를 숨겨도 표시합니다.
- Resume 또는 새 세션으로 재연결하면 현재 채널의 마지막으로 보던 기록 범위까지 페이지를 따라 다시 읽습니다. 누락된 새 메시지와 편집·삭제를 반영하고, 기록 조회 중 도착한 이벤트도 적용합니다.
- 복구 완료 후 전송을 다시 허용합니다. 기록 복구 실패 시 초안을 유지하며 `:sync`로 재시도할 수 있습니다. 전송 성공 여부가 불확실한 메시지는 자동 재전송하지 않습니다.
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
