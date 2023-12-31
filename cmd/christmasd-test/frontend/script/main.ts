import { ControllerSession } from "./christmasd.ts";
import { TreeCanvas } from "./tree.ts";

const treeCanvas = document.getElementById("tree") as HTMLCanvasElement;
const consoleElem = document.getElementById("console");

function writeToConsole(...nodes: (string | Node)[]) {
  const isBottomed =
    consoleElem.scrollHeight - consoleElem.scrollTop - consoleElem.clientHeight < 1;

  consoleElem.append("\n", ...nodes);

  if (isBottomed) {
    consoleElem.scrollTop = consoleElem.scrollHeight;
  }
}

function writeErrorToConsole(message: string) {
  const span = document.createElement("span");
  span.classList.add("error");
  span.innerText = message;
  writeToConsole(span);
}

function resetSession() {
  let session: ControllerSession;

  try {
    session = new ControllerSession();
  } catch (err) {
    writeErrorToConsole(`error connecting to server: ${err}`);
    return;
  }

  session.on("going_away", (ev) => {
    writeErrorToConsole(`server is going away, reason: ${ev.reason}`);
  });

  session.once("init", (ev) => {
    const ledPoints = ev.led_coords;
    const wsScheme = location.protocol === "https:" ? "wss" : "ws";
    const wsHost = location.host;
    const wsLink = `${wsScheme}://${wsHost}/ws/${ev.session_token}`;

    const tree = new TreeCanvas(treeCanvas, ledPoints);
    session.on("frame", (ev) => {
      tree.draw(ev.led_colors);
    });

    writeToConsole("Connected to server!");

    const a = document.createElement("a");
    a.href = wsLink;
    a.onclick = (ev) => {
      ev.preventDefault();
      navigator.clipboard.writeText(wsLink);
      writeToConsole(`Copied Websocket link to clipboard!`);
    };
    a.innerText = wsLink;

    const span = document.createElement("span");
    span.classList.add("ws-link");
    span.append("Point your script to ");
    span.appendChild(a);

    writeToConsole(span);
  });
}

resetSession();
