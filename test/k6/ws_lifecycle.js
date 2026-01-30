import ws from "k6/ws";
import { check, sleep } from "k6";

const WS_URL = __ENV.RTTY_WS_URL || "ws://localhost:8080/ws";
const ORIGIN = __ENV.RTTY_ORIGIN || "http://localhost:8080";

const resizeMessage = JSON.stringify({
  event: "resize",
  data: { cols: 80, rows: 24 },
});

const sendKeyMessage = JSON.stringify({
  event: "sendKey",
  data: "echo k6-lifecycle\n",
});

const closeMessage = JSON.stringify({
  event: "close",
  data: "",
});

export const options = {
  vus: 1,
  iterations: 1,
};

export default function () {
  let receivedData = false;
  const res = ws.connect(
    WS_URL,
    {
      headers: {
        Origin: ORIGIN,
      },
    },
    function (socket) {
      socket.on("open", function () {
        socket.send(resizeMessage);
        socket.send(sendKeyMessage);
        socket.setTimeout(function () {
          socket.close();
        }, 1000);
        sleep(0.2);
        socket.send(closeMessage);
      });

      socket.on("message", function () {
        receivedData = true;
      });

    }
  );

  check(res, {
    "upgrade status is 101": (r) => r && r.status === 101,
  });

  check(receivedData, {
    "received terminal output": (v) => v === true,
  });
}
