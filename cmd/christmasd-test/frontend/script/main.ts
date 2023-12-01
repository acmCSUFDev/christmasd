import { ControllerSession } from "./christmasd.ts";
import { TreeCanvas } from "./tree.ts";

const treeCanvas = document.getElementById("tree") as HTMLCanvasElement;
const consoleElem = document.getElementById("console");

function writeToConsole(...nodes: (string | Node)[]) {
  consoleElem.append("\n", ...nodes);
}

function writeErrorToConsole(message: string) {
  const span = document.createElement("span");
  span.classList.add("error");
  span.innerText = message;
  writeToConsole(span);
}

const session = new ControllerSession();
let tree: TreeCanvas;

session.once("init", (ev) => {
  const ledPoints = ev.led_coords;
  const wsLink = `ws://${location.host}/ws/${ev.session_token}`;

  tree = new TreeCanvas(treeCanvas, ledPoints);
  session.on("frame", (ev) => {
    tree.draw(ev.led_colors);
  });
  session.on("going_away", (ev) => {
    writeErrorToConsole(`Server is going away, reason: ${ev.reason}`);
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
