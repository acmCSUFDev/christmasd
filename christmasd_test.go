package christmasd

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/gobwas/ws/wsutil"
	"github.com/google/go-cmp/cmp"
	"github.com/neilotoole/slogt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"dev.acmcsuf.com/christmasd/christmaspb"
)

func TestSession(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		play   func(t *testing.T, conn io.ReadWriteCloser)
	}{
		{
			name: "invalid secret",
			play: func(t *testing.T, conn io.ReadWriteCloser) {
				writeClientMessage(t, conn, &christmaspb.LEDClientMessage{
					Message: &christmaspb.LEDClientMessage_Authenticate{
						Authenticate: &christmaspb.AuthenticateRequest{
							Secret: "bruh moment",
						},
					},
				})

				assertMessage(t, conn, &christmaspb.LEDServerMessage{
					Message: &christmaspb.LEDServerMessage_Authenticate{
						Authenticate: &christmaspb.AuthenticateResponse{
							Success: false,
						},
					},
				})

				assertMessage(t, conn, &christmaspb.LEDServerMessage{
					Error: proto.String("invalid secret"),
				})

				expectCloseFrame(t, conn)
			},
			config: Config{
				Secret: "test",
			},
		},
		{
			name: "valid secret",
			play: func(t *testing.T, conn io.ReadWriteCloser) {
				writeClientMessage(t, conn, &christmaspb.LEDClientMessage{
					Message: &christmaspb.LEDClientMessage_Authenticate{
						Authenticate: &christmaspb.AuthenticateRequest{
							Secret: "bruh moment",
						},
					},
				})

				assertMessage(t, conn, &christmaspb.LEDServerMessage{
					Message: &christmaspb.LEDServerMessage_Authenticate{
						Authenticate: &christmaspb.AuthenticateResponse{
							Success: true,
						},
					},
				})
			},
			config: Config{
				Secret: "bruh moment",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conn := startTestSession(t, ctx, test.config)
			test.play(t, conn)
		})
	}
}

func writeClientMessage(t *testing.T, conn io.ReadWriteCloser, msg *christmaspb.LEDClientMessage) {
	t.Helper()

	b, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal("invalid client proto message:", err)
	}
	if err := wsutil.WriteClientBinary(conn, b); err != nil {
		t.Fatal("error writing client message:", err)
	}
}

func readServerMessage(t *testing.T, conn io.ReadWriteCloser) *christmaspb.LEDServerMessage {
	t.Helper()

	b, err := wsutil.ReadServerBinary(conn)
	if err != nil {
		t.Fatal("error reading server message:", err)
	}

	msg := &christmaspb.LEDServerMessage{}
	if err := proto.Unmarshal(b, msg); err != nil {
		t.Fatal("invalid server proto message:", err)
	}

	return msg
}

func assertMessage(t *testing.T, conn io.ReadWriteCloser, expect *christmaspb.LEDServerMessage) {
	t.Helper()

	actual := readServerMessage(t, conn)
	assertEq(t, expect, actual)
}

func assertEq[T any](t *testing.T, expected, actual T, opts ...cmp.Option) {
	t.Helper()

	opts = append(opts, protocmp.Transform())
	if diff := cmp.Diff(expected, actual, opts...); diff != "" {
		t.Errorf("unexpected diff (-want +got):\n%s", diff)
	}
}

func expectCloseFrame(t *testing.T, conn io.ReadWriteCloser) {
	t.Helper()
	var closedErr wsutil.ClosedError

	_, op, err := wsutil.ReadServerData(conn)
	if err == nil {
		t.Fatal("no close frame received, got op", op)
	}
	if !errors.As(err, &closedErr) {
		t.Fatal("unexpected non-ClosedError while reading server data:", err)
	}

	// Responding close frame is automatically handled by gobwas/ws/wsutil.
	// See wsutil/handler.go @ ControlHandler.HandleClose.
}

func startTestSession(t *testing.T, ctx context.Context, cfg Config) io.ReadWriteCloser {
	t.Helper()

	conn1, conn2 := net.Pipe()

	t.Cleanup(func() {
		t.Log("closing test session pipes")
		conn1.Close()
		conn2.Close()
	})

	logger := slogt.New(t)

	session := &Session{
		ws:     newWebsocketServer(conn1, logger),
		logger: logger,
		cfg:    cfg,
	}

	ctx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)

	t.Cleanup(func() {
		cancel()
		if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
			t.Error("server session error:", err)
		}
	})

	go func() {
		errCh <- session.Start(ctx)
	}()

	return conn2
}
