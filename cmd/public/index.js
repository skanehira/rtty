const fontFamily = "{fontFamily}";
const fontSize = "{fontSize}";
const option = {
  cursorBlink: true,
  rendererType: "canvas",
};

if (fontFamily) {
  option["fontFamily"] = fontFamily;
}

if (fontSize) {
  option["fontSize"] = fontSize;
}

const terminal = new Terminal(option);
const fitAddon = new FitAddon.FitAddon();
const linkAddon = new WebLinksAddon.WebLinksAddon();

terminal.loadAddon(fitAddon);
terminal.loadAddon(linkAddon);
terminal.open(document.getElementById('terminal'));
terminal.focus();
fitAddon.fit();

let socket = null;
let connected = false;
let reconnectDelay = 250;
const reconnectDelayMax = 5000;
const reconnectFactor = 1.7;
let reconnectTimer = null;

// workaround
// for redraw terminal screen when reload window
const redraw = (socket, msg) => {
  msg.data.cols--
  terminal.resize(msg.data.cols, msg.data.rows)
  socket.send(JSON.stringify(msg));

  msg.data.cols++
  terminal.resize(msg.data.cols, msg.data.rows)
  socket.send(JSON.stringify(msg));
}

const sendMessage = (msg) => {
  if (!socket || socket.readyState !== WebSocket.OPEN) {
    return;
  }
  socket.send(JSON.stringify(msg));
}

const scheduleReconnect = () => {
  if (reconnectTimer) {
    return;
  }
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    connect();
    reconnectDelay = Math.min(reconnectDelayMax, Math.floor(reconnectDelay * reconnectFactor));
  }, reconnectDelay);
}

const connect = () => {
  socket = new WebSocket('ws://{addr}:{port}/ws');

  socket.onopen = () => {
    connected = true;
    reconnectDelay = 250;
    const msg = {
      event: "resize",
      data: {
        "cols": terminal.cols,
        "rows": terminal.rows,
      },
    };
    sendMessage(msg);
    redraw(socket, msg)
  }

  socket.onmessage = (e) => {
    terminal.write(e.data);
  }

  socket.onclose = () => {
    if (connected) {
      terminal.writeln('[Disconnected]');
    }
    connected = false;
    scheduleReconnect();
  }

  socket.onerror = () => {
    socket.close();
  }
}

terminal.onData(data => {
  if (!connected) {
    return;
  }
  const msg = {
    event: "sendKey",
    data: data,
  }
  sendMessage(msg);
})

terminal.onResize((size) => {
  terminal.resize(size.cols, size.rows);
  const msg = {
    event: "resize",
    data: {
      cols: size.cols,
      rows: size.rows,
    },
  }
  sendMessage(msg);
});

window.onbeforeunload = () => {
  if (socket) {
    socket.close();
  }
}

window.addEventListener("resize", () => {
  fitAddon.fit()
})

connect();
