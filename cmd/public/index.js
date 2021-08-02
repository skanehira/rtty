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

const socket = new WebSocket('ws://{addr}:{port}/ws');

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

socket.onopen = () => {
  const msg = {
    event: "resize",
    data: {
      "cols": terminal.cols,
      "rows": terminal.rows,
    },
  };
  socket.send(JSON.stringify(msg));

  redraw(socket, msg)

  terminal.onData(data => {
    switch (socket.readyState) {
      case WebSocket.CLOSED:
      case WebSocket.CLOSING:
        terminal.dispose();
        return;
    }
    const msg = {
      event: "sendKey",
      data: data,
    }
    socket.send(JSON.stringify(msg));
  })

  socket.onclose = () => {
    terminal.writeln('[Disconnected]');
  }

  socket.onmessage = (e) => {
    terminal.write(e.data);
  }

  terminal.onResize((size) => {
    terminal.resize(size.cols, size.rows);
    const msg = {
      event: "resize",
      data: {
        cols: size.cols,
        rows: size.rows,
      },
    }
    socket.send(JSON.stringify(msg));
  });

  window.onbeforeunload = () => {
    socket.close();
  }

  window.addEventListener("resize", () => {
    fitAddon.fit()
  })
}

