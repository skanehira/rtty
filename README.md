![GitHub Repo stars](https://img.shields.io/github/stars/skanehira/rtty?style=social)
![GitHub](https://img.shields.io/github/license/skanehira/rtty)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/skanehira/rtty)
![GitHub all releases](https://img.shields.io/github/downloads/skanehira/rtty/total)
![GitHub CI Status](https://img.shields.io/github/workflow/status/skanehira/rtty/ci?label=CI)
![GitHub Release Status](https://img.shields.io/github/workflow/status/skanehira/rtty/Release?label=release)

# rtty
Terminal on browser via websocket

## Supportted OS
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
$ rtty run
2021/05/10 13:35:23 start server with port: 9999

# Help
$ rtty
Usage:
  rtty [flags]
  rtty [command]

Available Commands:
  help        Help about any command
  run         Run server
  version     Version of rtty

Flags:
  -h, --help   help for rtty

Use "rtty [command] --help" for more information about a command.
```

## Author
skanehira
