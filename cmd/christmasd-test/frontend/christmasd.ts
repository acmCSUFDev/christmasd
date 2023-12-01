import * as event from "https://deno.land/x/event@2.0.1/mod.ts";

export type RGB = `#${string}`;
export type Point = { X: number; Y: number };

export type InitEvent = {
  led_coords: Point[];
  session_token: string;
};

export type ErrorEvent = {
  message: string;
};

export type FrameEvent = {
  frame: RGB[];
};

export type SSEEvents = {
  init: [InitEvent];
  error: [ErrorEvent];
  frame: [FrameEvent];
};

export class ControllerHandler extends event.EventEmitter<SSEEvents> {
}
