package main

import (
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"

	"dev.acmcsuf.com/christmas/lib/xcolor"
)

// ControllerEvent describes an exchanging SSE event with the
// controller side.
type ControllerEvent interface {
	Type() ControllerEventType
}

// ControllerEventType is a type of message sent to the controller.
type ControllerEventType string

const (
	ControllerEventTypeInit      ControllerEventType = "init"
	ControllerEventTypeFrame     ControllerEventType = "frame"
	ControllerEventTypeGoingAway ControllerEventType = "going_away"
)

// ControllerInit is the init message sent to the controller.
type ControllerInit struct {
	LEDCoords    []image.Point `json:"led_coords"`
	SessionToken string        `json:"session_token"`
}

func (ControllerInit) Type() ControllerEventType {
	return ControllerEventTypeInit
}

// ControllerFrame is the frame message sent to the controller.
// It contains the frame data.
type ControllerFrame struct {
	LEDColors []xcolor.RGB `json:"led_colors"`
}

func (ControllerFrame) Type() ControllerEventType {
	return ControllerEventTypeFrame
}

// ControllerGoingAway is the going_away message sent to the controller.
// It contains the reason for the going away.
type ControllerGoingAway struct {
	Reason string `json:"reason"`
}

func (ControllerGoingAway) Type() ControllerEventType {
	return ControllerEventTypeGoingAway
}

type sseEvent struct {
	Type string
	Data any
}

type writeFlusher interface {
	io.Writer
	http.Flusher
}

func writeSSE(w writeFlusher, ev sseEvent) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, ev.Data)
	w.Flush()
}

func controllerEventSSE(event ControllerEvent) sseEvent {
	b, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	return sseEvent{
		Type: string(event.Type()),
		Data: b,
	}
}
