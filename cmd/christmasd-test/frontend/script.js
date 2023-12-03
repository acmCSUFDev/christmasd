// deno-fmt-ignore-file
// deno-lint-ignore-file
// This code was bundled using `deno bundle` and it's not recommended to edit it manually

const isNullish = (value)=>value === null || value === undefined;
class EventEmitter {
    #listeners = {};
    #globalWriters = [];
    #onWriters = {};
    #limit;
    constructor(maxListenersPerEvent){
        this.#limit = maxListenersPerEvent ?? 10;
    }
    on(eventName, listener) {
        if (listener) {
            if (!this.#listeners[eventName]) {
                this.#listeners[eventName] = [];
            }
            if (this.#limit !== 0 && this.#listeners[eventName].length >= this.#limit) {
                throw new TypeError("Listeners limit reached: limit is " + this.#limit);
            }
            this.#listeners[eventName].push({
                once: false,
                cb: listener
            });
            return this;
        } else {
            if (!this.#onWriters[eventName]) {
                this.#onWriters[eventName] = [];
            }
            if (this.#limit !== 0 && this.#onWriters[eventName].length >= this.#limit) {
                throw new TypeError("Listeners limit reached: limit is " + this.#limit);
            }
            const { readable, writable } = new TransformStream();
            this.#onWriters[eventName].push(writable.getWriter());
            return readable[Symbol.asyncIterator]();
        }
    }
    once(eventName, listener) {
        if (!this.#listeners[eventName]) {
            this.#listeners[eventName] = [];
        }
        if (this.#limit !== 0 && this.#listeners[eventName].length >= this.#limit) {
            throw new TypeError("Listeners limit reached: limit is " + this.#limit);
        }
        if (listener) {
            this.#listeners[eventName].push({
                once: true,
                cb: listener
            });
            return this;
        } else {
            return new Promise((res)=>{
                this.#listeners[eventName].push({
                    once: true,
                    cb: (...args)=>res(args)
                });
            });
        }
    }
    async off(eventName, listener) {
        if (!isNullish(eventName)) {
            if (listener) {
                this.#listeners[eventName] = this.#listeners[eventName]?.filter(({ cb })=>cb !== listener);
            } else {
                if (this.#onWriters[eventName]) {
                    for (const writer of this.#onWriters[eventName]){
                        await writer.close();
                    }
                    delete this.#onWriters[eventName];
                }
                delete this.#listeners[eventName];
            }
        } else {
            for (const writers of Object.values(this.#onWriters)){
                for (const writer of writers){
                    await writer.close();
                }
            }
            this.#onWriters = {};
            for (const writer of this.#globalWriters){
                await writer.close();
            }
            this.#globalWriters = [];
            this.#listeners = {};
        }
        return this;
    }
    async emit(eventName, ...args) {
        const listeners = this.#listeners[eventName]?.slice() ?? [];
        for (const { cb, once } of listeners){
            cb(...args);
            if (once) {
                this.off(eventName, cb);
            }
        }
        if (this.#onWriters[eventName]) {
            for (const writer of this.#onWriters[eventName]){
                await writer.write(args);
            }
        }
        for (const writer of this.#globalWriters){
            await writer.write({
                name: eventName,
                value: args
            });
        }
    }
    [Symbol.asyncIterator]() {
        if (this.#limit !== 0 && this.#globalWriters.length >= this.#limit) {
            throw new TypeError("Listeners limit reached: limit is " + this.#limit);
        }
        const { readable, writable } = new TransformStream();
        this.#globalWriters.push(writable.getWriter());
        return readable[Symbol.asyncIterator]();
    }
}
class ControllerSession extends EventEmitter {
    sse;
    constructor(){
        super();
        this.sse = new EventSource("/session");
        this.sse.addEventListener("init", (ev)=>this.emit("init", JSON.parse(ev.data)));
        this.sse.addEventListener("frame", (ev)=>this.emit("frame", JSON.parse(ev.data)));
        this.sse.addEventListener("going_away", (ev)=>this.emit("going_away", JSON.parse(ev.data)));
    }
}
class TreeCanvas {
    canvas;
    canvasWidth;
    canvasHeight;
    colors;
    ledPoints;
    ledScale;
    constructor(canvas, ledPoints){
        this.colors = Array(ledPoints.length).fill("#FFFFFF");
        this.canvas = canvas.getContext("2d");
        this.canvasWidth = canvas.width;
        this.canvasHeight = canvas.height;
        const ledMinX = Math.min(...ledPoints.map((p)=>p.X));
        const ledMaxX = Math.max(...ledPoints.map((p)=>p.X));
        const ledMinY = Math.min(...ledPoints.map((p)=>p.Y));
        const ledMaxY = Math.max(...ledPoints.map((p)=>p.Y));
        const ledWidth = ledMaxX - ledMinX;
        const ledHeight = ledMaxY - ledMinY;
        this.ledScale = Math.min(this.canvasWidth / ledWidth, this.canvasHeight / ledHeight);
        const ledOffsetX = (this.canvasWidth - ledWidth * this.ledScale) / 2;
        const ledOffsetY = (this.canvasHeight - ledHeight * this.ledScale) / 2;
        this.ledPoints = ledPoints.map((p)=>({
                X: (p.X - ledMinX) * this.ledScale + ledOffsetX,
                Y: (p.Y - ledMinY) * this.ledScale + ledOffsetY
            }));
        this.redraw();
    }
    draw(colors) {
        this.colors = colors;
        this.redraw();
    }
    redraw() {
        this.canvas.clearRect(0, 0, this.canvasWidth, this.canvasHeight);
        for (const [i, color] of this.colors.entries()){
            const { X, Y } = this.ledPoints[i];
            this.canvas.beginPath();
            this.canvas.arc(X, Y, 2, 0, 2 * Math.PI);
            this.canvas.fillStyle = color;
            this.canvas.fill();
        }
    }
}
const treeCanvas = document.getElementById("tree");
const consoleElem = document.getElementById("console");
function writeToConsole(...nodes) {
    consoleElem.append("\n", ...nodes);
}
function writeErrorToConsole(message) {
    const span = document.createElement("span");
    span.classList.add("error");
    span.innerText = message;
    writeToConsole(span);
}
const session = new ControllerSession();
let tree;
session.once("init", (ev)=>{
    const ledPoints = ev.led_coords;
    const wsScheme = location.protocol === "https:" ? "wss" : "ws";
    const wsHost = location.host;
    const wsLink = `${wsScheme}://${wsHost}/ws/${ev.session_token}`;
    tree = new TreeCanvas(treeCanvas, ledPoints);
    session.on("frame", (ev)=>{
        tree.draw(ev.led_colors);
    });
    session.on("going_away", (ev)=>{
        writeErrorToConsole(`Server is going away, reason: ${ev.reason}`);
    });
    writeToConsole("Connected to server!");
    const a = document.createElement("a");
    a.href = wsLink;
    a.onclick = (ev)=>{
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
