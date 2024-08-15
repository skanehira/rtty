![GitHub Repo stars](https://img.shields.io/github/stars/skanehira/rtty?style=social)
![GitHub](https://img.shields.io/github/license/skanehira/rtty)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/skanehira/rtty)
![GitHub all releases](https://img.shields.io/github/downloads/skanehira/rtty/total)
![GitHub CI Status](https://img.shields.io/github/actions/workflow/status/skanehira/rtty/ci.yaml?branch=main&label=CI)
![GitHub Release Status](https://img.shields.io/github/actions/workflow/status/skanehira/rtty/release.yaml?branch=main&label=Release)

# rtty
Terminal on browser via websocket

![](https://i.gyazo.com/bc8a484cdbffbf8fd1d6a574f181cb24.png)

## Supported OS
- Linux
- Mac

## Installation
- Build from source(Go v1.23.0 ~)
  ```sh
  go install github.com/skanehira/rtty@latest
  ```
- Download from Releases

## Usage
```sh
# Run server
$ rtty run zsh -p 8080 -v --font "Cica Regular" --font-size 20
2024/08/15 23:39:37 allowed origins [localhost:8080]
2024/08/15 23:39:37 running command: zsh
2024/08/15 23:39:37 running http://localhost:8080

# Help
$ rtty run -h
Run command

Usage:
  rtty run [command] [flags]

Command
  Execute specified command (default "/bin/zsh")

Flags:
  -a, --addr string                server address (default "localhost")
      --allow-origin stringArray   allow origin (default ["localhost:9999"])
      --font string                font
      --font-size string           font size
  -h, --help                       help for run
  -p, --port int                   server port (default 9999)
  -v, --view                       open browser
```

## Author
skanehira
