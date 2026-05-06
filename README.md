# sshfwd

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

```sh
go install github.com/lchen/sshfwd@latest
```

Or build from source:

```sh
git clone <this repo> && cd sshfwd
go build -o sshfwd .
```

## Run

```sh
sshfwd
```

That's it. Hosts come from `~/.ssh/config`.

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

`sshfwd` shells out to your system `ssh` binary using the
[ControlMaster](https://man.openbsd.org/ssh_config#ControlMaster) pattern,
so it inherits everything from `~/.ssh/config` — keys, jump hosts, agent
forwarding — for free. No SSH protocol code in Go.

State (which forwards were active per host) is persisted to
`~/.config/sshfwd/state.json` (or `%APPDATA%\sshfwd\state.json` on Windows)
so tunnels can be re-established after a restart.

## Requirements

- Go 1.21+
- An `ssh` binary that supports `ControlMaster` (OpenSSH on macOS / Linux / WSL).
  On native Windows, ControlMaster is unsupported and `sshfwd` will warn.
