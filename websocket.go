package christmasd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"dev.acmcsuf.com/christmasd/christmaspb"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
)

type closeFrame struct {
	Code   ws.StatusCode
	Reason string
}

func (f closeFrame) encode() []byte {
	return ws.NewCloseFrameBody(f.Code, f.Reason)
}

type websocketServer struct {
	// Messages is a channel of messages received from the client.
	Messages chan *christmaspb.LEDClientMessage
	// Sending is a channel of messages to send to the client.
	Sending chan *christmaspb.LEDServerMessage

	wsconn io.ReadWriteCloser
	logger *slog.Logger
}

func newWebsocketServer(wsconn io.ReadWriteCloser, logger *slog.Logger) *websocketServer {
	return &websocketServer{
		Messages: make(chan *christmaspb.LEDClientMessage),
		Sending:  make(chan *christmaspb.LEDServerMessage),

		wsconn: wsconn,
		logger: logger,
	}
}

// Send sends a message to the client.
func (s *websocketServer) Send(ctx context.Context, msg *christmaspb.LEDServerMessage) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.Sending <- msg:
		return nil
	}
}

// SendError sends an error message to the client. It is a convenience
// wrapper around Send. The server will automatically close the connection
// after sending the error.
func (s *websocketServer) SendError(ctx context.Context, err error) error {
	return s.Send(ctx, &christmaspb.LEDServerMessage{
		Error: proto.String(err.Error()),
	})
}

func (s *websocketServer) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errg, ctx := errgroup.WithContext(ctx)

	errg.Go(func() error {
		<-ctx.Done()

		s.logger.DebugContext(ctx,
			"closing websocket",
			"error", ctx.Err().Error())

		if closeErr := s.wsconn.Close(); closeErr != nil {
			s.logger.WarnContext(ctx,
				"failed to close websocket",
				"error", closeErr.Error())

			return fmt.Errorf("failed to close websocket: %w", closeErr)
		}

		return nil
	})

	errg.Go(func() error {
		defer cancel()

		var buf bytes.Buffer
		buf.Grow(1024)

		for {
			_, err := wsReadData(&buf, s.wsconn, ws.StateServerSide, ws.OpBinary)
			if err != nil {
				var closedErr wsutil.ClosedError
				if errors.As(err, &closedErr) {
					s.logger.DebugContext(ctx,
						"received close frame from client")

					return nil
				}

				if ctx.Err() != nil {
					return ctx.Err()
				}

				s.logger.DebugContext(ctx,
					"failed to read from websocket",
					"error", err.Error())

				return fmt.Errorf("failed to read from websocket: %w", err)
			}

			var msg christmaspb.LEDClientMessage
			if err := proto.Unmarshal(buf.Bytes(), &msg); err != nil {
				err = fmt.Errorf("failed to unmarshal message: %w", err)
				err = s.SendError(ctx, err)
				return err
			}

			// s.logger.DebugContext(ctx,
			// 	"received message from client",
			// 	"message", msg.String())

			select {
			case <-ctx.Done():
				return ctx.Err()
			case s.Messages <- &msg:
			}
		}
	})

	errg.Go(func() error {
		var marshaler proto.MarshalOptions

		var err error
		buf := make([]byte, 0, 1024)

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()

			case msg := <-s.Sending:
				buf = buf[:0]

				buf, err = marshaler.MarshalAppend(buf, msg)
				if err != nil {
					return fmt.Errorf("failed to marshal message: %w", err)
				}

				s.logger.DebugContext(ctx,
					"sending message to client",
					"message", msg.String())

				if err := wsutil.WriteServerBinary(s.wsconn, buf); err != nil {
					return fmt.Errorf("failed to write to websocket: %w", err)
				}

				// If we've just delivered an error, then shut down the
				// connection.
				if msg.Error != nil {
					closeFrame := closeFrame{
						Code:   ws.StatusNormalClosure,
						Reason: "error delivered to client",
					}

					s.logger.DebugContext(ctx,
						"sending close frame to client",
						"code", closeFrame.Code,
						"reason", closeFrame.Reason)

					if err := ws.WriteFrame(s.wsconn, ws.NewCloseFrame(closeFrame.encode())); err != nil {
						s.logger.WarnContext(ctx,
							"failed to write close frame",
							"error", err.Error())
					} else {
						s.logger.DebugContext(ctx,
							"close frame sent")
					}

					// Give 2 seconds for the close frame to be sent, then we'll
					// forcefully stop the context to close the connection.
					errg.Go(func() error {
						timer := time.NewTimer(2 * time.Second)
						defer timer.Stop()

						select {
						case <-timer.C:
							cancel()
						case <-ctx.Done():
						}
						return nil
					})

					// Exit.
					return nil
				}
			}
		}
	})

	return errg.Wait()
}

func wsReadData(dst *bytes.Buffer, src io.ReadWriter, s ws.State, want ws.OpCode) (ws.OpCode, error) {
	controlHandler := wsutil.ControlFrameHandler(src, s)
	rd := wsutil.Reader{
		Source:          src,
		State:           s,
		SkipHeaderCheck: false,
		OnIntermediate:  controlHandler,
	}
	for {
		hdr, err := rd.NextFrame()
		if err != nil {
			return 0, err
		}
		if hdr.OpCode.IsControl() {
			if err := controlHandler(hdr, &rd); err != nil {
				return 0, err
			}
			continue
		}
		if hdr.OpCode&want == 0 {
			if err := rd.Discard(); err != nil {
				return 0, err
			}
			continue
		}

		dst.Reset()
		_, err = io.Copy(dst, &rd)
		return hdr.OpCode, err
	}
}
