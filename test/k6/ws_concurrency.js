import ws from "k6/ws";
import { check, sleep } from "k6";

const WS_URL = __ENV.RTTY_WS_URL || "ws://localhost:8080/ws";
const ORIGIN = __ENV.RTTY_ORIGIN || "http://localhost:8080";
const scenarioFilter = __ENV.RTTY_SCENARIO || "all";

const resizeMessage = JSON.stringify({
  event: "resize",
  data: { cols: 80, rows: 24 },
});

const sendKeyMessage = JSON.stringify({
  event: "sendKey",
  data: "echo k6-concurrency\n",
});

const duration = __ENV.RTTY_CONCURRENCY_DURATION || "30s";

function buildScenarios() {
  const scenarios = {};
  if (scenarioFilter === "all" || scenarioFilter === "100") {
    scenarios.vu_100 = {
      executor: "constant-vus",
      vus: Number(__ENV.RTTY_VUS_100 || 100),
      duration,
    };
  }
  if (scenarioFilter === "all" || scenarioFilter === "500") {
    scenarios.vu_500 = {
      executor: "constant-vus",
      vus: Number(__ENV.RTTY_VUS_500 || 500),
      duration,
    };
  }
  if (scenarioFilter === "all" || scenarioFilter === "1000") {
    scenarios.vu_1000 = {
      executor: "constant-vus",
      vus: Number(__ENV.RTTY_VUS_1000 || 1000),
      duration,
    };
  }
  return scenarios;
}

export const options = {
  scenarios: buildScenarios(),
  thresholds: {
    checks: ["rate>0.97"],
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
        sleep(0.1);
        socket.close();
      });
    }
  );

  check(res, {
    "upgrade status is 101": (r) => r && r.status === 101,
  });
}
