# Mison — Original Project Brief (frozen)

> This is the original planning document, preserved as reference.
> Authoritative specs live in DESIGN.md; this file is not updated.

## 1. Project Overview

Mison은 여러 컴퓨터에서 동일한 개발환경을 쉽게 재현하고 동기화하기 위한 CLI 도구다.

핵심 아이디어는 기존의 강력한 개발 도구인 `mise`를 내부 엔진으로 사용하고, 사용자는 `mise` 자체를 직접 관리하지 않아도 되도록 추상화하는 것이다.

Mison의 목표는 다음과 같다.

> 한 번 구성한 개발환경을 GitHub에 저장하고, 다른 macOS/Linux 환경에서 명령 하나로 동일한 개발환경을 재현한다.

Mison은 새로운 패키지 매니저를 만드는 것이 아니다. `mise`를 개발환경 관리 엔진으로 사용하고 다음 작업을 자동화하는 상위 레이어다.

- mise 설치 / 초기화 / 업데이트
- 개발환경 구성 관리
- GitHub 저장소와 환경 구성 동기화
- 현재 OS / architecture에 맞는 tool 설치
- 새로운 머신의 bootstrap
- 현재 머신의 환경을 선언적 구성으로 가져오기 (adopt — V1 제외)

## 2. Problem

새 Linux 머신을 사용할 때마다 node/python/go/neovim 등을 다시 설치하고 설정을 복원해야 한다. 특히 macOS와 Linux는 설치 방법과 architecture가 다르기 때문에 단순한 파일 복사만으로는 해결하기 어렵다. Mison은 이 문제를 해결한다.

## 3. Core Philosophy

### 3.1 Desired State

"현재 컴퓨터에 무엇이 설치되어 있는가"보다 "내 개발환경이 어떻게 구성되어 있어야 하는가"를 중요하게 생각한다. GitHub에 저장되는 환경 설정이 source of truth다.

```toml
[tools]
node = "22"
python = "3.13"
go = "1.25"
opencode = "latest"
neovim = "latest"
```

각 머신은 이 선언을 가져와 자신의 OS와 architecture에 맞는 실제 환경으로 만든다.

### 3.2 mise as Engine

Mison은 자체적으로 모든 tool installer를 구현하지 않는다. 가능한 경우 `mise`에 설치와 버전 관리를 위임한다. macOS와 Linux에서 package manager가 다르더라도 mise가 지원하는 tool이라면 Mison이 직접 OS별 설치 로직을 구현할 필요가 없다.

## 4. Target Platforms

- macOS / Linux
- Apple Silicon (arm64) / x86_64 (amd64)
- Windows는 V1 범위에서 제외

## 5. User Experience

사용자가 `mise`를 직접 알 필요가 없도록 만드는 것이 가장 중요한 목표다.

```text
$ mison init

✓ Detecting system
✓ Installing mise
✓ Configuring mise
✓ Connecting GitHub
✓ Creating environment
✓ Syncing environment

Environment ready.
```

## 6. Core Commands (V1 확정: 5개 — scan/adopt 제외)

- `mison init` — mise 설치, gh 인증, 환경 저장소 생성/연결
- `mison install <tools...>` — 선언 추가 + 즉시 설치 + 자동 push
- `mison uninstall <tools...>` — 선언 제거 + 로컬 제거 + 자동 push
- `mison sync` — GitHub pull → OS/arch 감지 → mise install 적용
- `mison status` — 선언 vs 설치 상태 비교 (✓/✗/⚠)

(원문의 add/remove는 install/uninstall으로, scan/adopt는 V2로 이동)

## 7-8. GitHub Integration & Sync Model

GitHub는 환경 정의를 저장하는 중앙 저장소 역할을 한다. 핵심 데이터는 `mise.toml`이다. 동일한 환경 선언을 macOS와 Linux에서 사용하고 OS별 실제 설치 방법은 mise에 위임한다.

## 9. mise Lifecycle

Mison이 mise 자체의 lifecycle을 관리한다. 사용자는 별도로 mise를 설치할 필요가 없다. 업데이트 정책: 명시적 업데이트만.

## 10. Important Distinction

Mison은 모든 Linux system package를 관리하는 것을 목표로 하지 않는다. 대상: CLI tools, runtimes, formatters, linters, LSPs, AI coding tools. 비대상: GPU drivers, kernel, system libraries, Docker, GUI apps.

## 11. Provider Architecture

장기적으로 여러 설치 backend 지원 가능하나 V1은 mise provider만 구현한다. V1에서 패키지 매니저를 직접 추상화하지 않는다.

## 12. Architecture

```
mison
├── CLI
├── Environment Manager
├── GitHub Manager
├── mise Manager
├── System Detector (OS / Architecture / Shell)
└── Future Providers
```

## 13. V1 Scope

Must Have: macOS/Linux, mise 자동 설치/업데이트, GitHub 연동, 환경 repository 생성/연결, 5개 명령, mise 기반 tool 설치, OS/architecture detection.

Out of Scope: Windows, 자체 package manager, apt/brew/dnf abstraction, GUI, Web dashboard, Cloud backend, 자체 tool registry, secrets, dotfiles (향후 chezmoi 연동 검토).

## 14. Example Workflow (First Milestone)

### Mac (Machine A)

```bash
mison init
mison install node opencode
# GitHub mise.toml: node = "22", opencode = "latest"
```

### Linux (Machine B)

```bash
mison sync
# ✓ mise installed ✓ environment fetched ✓ node 22 installed ✓ opencode installed
```

사용자는 Linux의 apt/dnf/pacman 차이를 신경 쓰지 않는다.

## 15. Design Principles

1. Hide mise complexity — 사용자가 mise의 존재를 몰라도 사용 가능
2. Reuse existing tools — tool installation/version management는 mise에 위임
3. Git is the source of truth — 환경의 원하는 상태는 GitHub에 저장
4. Reproducibility — 같은 선언을 다른 머신에서 재현 가능
5. Minimal V1 — 먼저 Mison → mise → Development tools 구조를 안정화

## 16. Open Questions → 해결됨 (DESIGN.md decisions log 참조)

1. GitHub auth: mise → gh 설치 → device flow ✓
2. Repository 전략: 자동 생성 private ✓
3. mise 설치: 공식 installer (mise.run) ✓
4. mise 업데이트: 명시적만 ✓
5. 환경 파일: mise.toml 직접 사용 ✓
6. adopt 방식: V2로 연기 ✓
7. commit/push 자동화: install/uninstall이 자동 ✓
8. 충돌 처리: fetch+rebase + 의미론적 merge ✓
9. 머신별 설정: mise os 필드 (union semantics) ✓
10. secrets: V1 범위 외 ✓

## 19. Project Positioning

```
chezmoi  → dotfiles/configuration
mise     → development tools/version management
GitHub   → environment storage/version control
Mison    → bootstrap + orchestration + synchronization
```

> Mison is the synchronization and bootstrap layer for reproducible development environments.
