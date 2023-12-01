import * as event from "https://deno.land/x/event@2.0.1/mod.ts";

export type RGB = `#${string}`;
export type Point = { X: number; Y: number };

export type InitEvent = {
  led_coords: Point[];
  session_token: string;
};

export type FrameEvent = {
  led_colors: RGB[];
};

export type GoingAwayEvent = {
  reason: string;
};

export type SSEEvents = {
  init: [InitEvent];
  frame: [FrameEvent];
  going_away: [GoingAwayEvent];
};

export class ControllerSession extends event.EventEmitter<SSEEvents> {
  private sse: EventSource;

  constructor() {
    super();
    this.sse = new EventSource("/session");
    this.sse.addEventListener("init", (ev) => this.emit("init", JSON.parse(ev.data)));
    this.sse.addEventListener("frame", (ev) => this.emit("frame", JSON.parse(ev.data)));
    this.sse.addEventListener("going_away", (ev) => this.emit("going_away", JSON.parse(ev.data)));
  }
}
