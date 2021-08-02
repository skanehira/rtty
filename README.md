![GitHub Repo stars](https://img.shields.io/github/stars/skanehira/rtty?style=social)
![GitHub](https://img.shields.io/github/license/skanehira/rtty)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/skanehira/rtty)
![GitHub all releases](https://img.shields.io/github/downloads/skanehira/rtty/total)
![GitHub CI Status](https://img.shields.io/github/workflow/status/skanehira/rtty/ci?label=CI)
![GitHub Release Status](https://img.shields.io/github/workflow/status/skanehira/rtty/Release?label=release)

# rtty
Terminal on browser via websocket

![](https://i.gyazo.com/bc8a484cdbffbf8fd1d6a574f181cb24.png)

## Supported OS
- Linux
- Mac

## Installation
- Build from source(Go v1.16 ~)
  ```sh
  go install github.com/skanehira/rtty@latest
  ```
- Download from Releases

## Usage
```sh
# Run server
$ rtty run zsh -p 8080 -v --font "Cica Regular" --font-size 20
2021/05/10 14:08:11 running command: zsh
2021/05/10 14:08:11 running http://localhost:8080

# Help
$ rtty run -h
Run command

Usage:
  rtty run [command] [flags]

Command
  Execute specified command (default $SHELL)

Flags:
  -a, --addr string        server address
      --font string        font
      --font-size string   font size
  -h, --help               help for run
  -p, --port int           server port (default 9999)
  -v, --view               open browser
```

## Author
skanehira
