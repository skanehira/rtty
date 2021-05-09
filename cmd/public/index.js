const terminal = new Terminal({ cursorBlink: true });
const fitAddon = new FitAddon.FitAddon();
terminal.loadAddon(fitAddon);
terminal.open(document.getElementById('terminal'))
fitAddon.fit();

const elm = document.querySelector(".terminal")

const socket = new WebSocket('ws://localhost:3000/ws');
socket.onopen = () => {
  const initData = {
    "window_size": {
      "width": elm.clientWidth,
      "height": elm.clientHeight
    },
    "cmd": "zsh"
  };

  socket.send(JSON.stringify(initData));
  terminal.onKey((e) => {
    socket.send(e.key);
  });

  socket.onclose = () => {
    terminal.write('\r\n[Disconnected]\r\n')
  }

  socket.onmessage = (e) => {
    console.log(e);
    terminal.write(e.data);
  }
}
