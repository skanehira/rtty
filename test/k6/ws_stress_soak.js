import ws from "k6/ws";
import { check, sleep } from "k6";

const WS_URL = __ENV.RTTY_WS_URL || "ws://localhost:8080/ws";
const ORIGIN = __ENV.RTTY_ORIGIN || "http://localhost:8080";

const stressMaxVus = Number(__ENV.RTTY_STRESS_MAX_VUS || 200);
const stressDuration = __ENV.RTTY_STRESS_DURATION || "3m";
const soakVus = Number(__ENV.RTTY_SOAK_VUS || 50);
const soakDuration = __ENV.RTTY_SOAK_DURATION || "1h";

const resizeMessage = JSON.stringify({
  event: "resize",
  data: { cols: 80, rows: 24 },
});

const sendKeyMessage = JSON.stringify({
  event: "sendKey",
  data: "echo k6-stress\n",
});

export const options = {
  scenarios: {
    stress: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "30s", target: Math.max(1, Math.floor(stressMaxVus * 0.25)) },
        { duration: "30s", target: Math.max(1, Math.floor(stressMaxVus * 0.6)) },
        { duration: "30s", target: stressMaxVus },
        { duration: "30s", target: 0 },
      ],
      gracefulRampDown: "30s",
    },
    soak: {
      executor: "constant-vus",
      vus: soakVus,
      duration: soakDuration,
    },
  },
  thresholds: {
    checks: ["rate>0.95"],
  },
};

export default function () {
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
        sleep(0.2);
        socket.close();
      });
    }
  );

  check(res, {
    "upgrade status is 101": (r) => r && r.status === 101,
  });
}
