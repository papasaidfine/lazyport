# lazyport

A terminal UI for managing SSH port forwards, inspired by VSCode's Ports panel.
Pick a host, type a port, hit Enter — the tunnel is live.

```
┌─ Hosts ──────────┐┌─ Tunnels: dev-server ──────────────────┐
│ > dev-server   ● ││ PORT    STATUS                          │
│   staging      ○ ││ 8080    active   [del]                  │
│   prod         ○ ││ 5432    active   [del]                  │
│   bastion      ○ ││                                         │
│                  ││ Add port: [    ]  press Enter to forward│
└──────────────────┘└─────────────────────────────────────────┘
tab/←→ pane  ·  ↑↓ navigate  ·  enter connect/disconnect  ·  q quit
```

## Install

**Linux / macOS** — one-liner that downloads the right binary, verifies its
SHA-256, and drops it into `/usr/local/bin` (falls back to `~/.local/bin` if
that's not writable):

```sh
curl -fsSL https://raw.githubusercontent.com/papasaidfine/lazyport/main/install.sh | bash
```

**Windows** (PowerShell 5.1+):

```powershell
irm https://raw.githubusercontent.com/papasaidfine/lazyport/main/install.ps1 | iex
```

Override `INSTALL_DIR` (or `$env:INSTALL_DIR`) to install elsewhere. Override
`VERSION` (or `$env:VERSION`) to pin a specific tag, e.g. `VERSION=v0.1.0-beta.1`.

You can also grab a tarball / zip directly from the
[Releases page](https://github.com/papasaidfine/lazyport/releases) and put the
binary on your `PATH` yourself.

### From source

```sh
go install github.com/papasaidfine/lazyport@latest
# or
git clone https://github.com/papasaidfine/lazyport && cd lazyport && go build -o lazyport .
```

## Run

```sh
lazyport
```

That's it. Hosts come from `~/.ssh/config`.

## Themes

```sh
lazyport --theme dracula        # one-shot
LAZYPORT_THEME=dracula lazyport # via env (handy for shell rc files)
```

Built-in themes: `nord` (default, https://www.nordtheme.com/) and `dracula`
(https://draculatheme.com/). Adding more is a one-struct addition in
[`ui/theme.go`](ui/theme.go) — PRs welcome.

## Keys

| Key                 | Action                                |
|---------------------|---------------------------------------|
| `Tab` / `←` `→`     | Switch pane                           |
| `↑` `↓` / `j` `k`   | Navigate list                         |
| `Enter` (host)      | Connect / disconnect host             |
| `i` or `/`          | Jump to port input                    |
| `Enter` (input)     | Forward `localhost:N → host:N`        |
| `d` / `x`           | Delete selected tunnel                |
| `q` / `Ctrl+C`      | Quit (asks whether to tear down)      |

## How it works

`lazyport` shells out to your system `ssh` binary using the
[ControlMaster](https://man.openbsd.org/ssh_config#ControlMaster) pattern,
so it inherits everything from `~/.ssh/config` — keys, jump hosts, agent
forwarding — for free. No SSH protocol code in Go.

State (which forwards were active per host) is persisted to
`~/.config/lazyport/state.json` (or `%APPDATA%\lazyport\state.json` on Windows)
so tunnels can be re-established after a restart.

## Requirements

- An `ssh` binary on `PATH` (OpenSSH on macOS / Linux / WSL / Windows).
- Key-based auth (ssh-agent or unencrypted key) — `lazyport` shells out to
  `ssh` and can't proxy a password prompt through the TUI.
- Go 1.21+ — only if you're building from source.

### How forwards are managed

On macOS and Linux, `lazyport` uses OpenSSH's
[ControlMaster](https://man.openbsd.org/ssh_config#ControlMaster) pattern:
one long-lived `ssh` process per host with a unix-socket control channel,
through which forwards are added and removed without re-authenticating.

On Windows, where ControlMaster's unix-socket plumbing isn't reliably
supported, each forward runs as its own `ssh -N -L ...` subprocess instead.
Same UX, just slightly slower per-forward (one auth per port rather than one
per host). Set `LAZYPORT_NO_CONTROLMASTER=1` to force this mode on any
platform — useful if your home directory is on NFS or another filesystem
where unix sockets misbehave.
