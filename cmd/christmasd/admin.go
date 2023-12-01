package main

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"

	"dev.acmcsuf.com/christmasd"
	"github.com/go-chi/chi/v5"
	"github.com/gofrs/uuid/v5"
	"libdb.so/hrt"
)

type adminHandler struct {
	*chi.Mux
	server *christmasd.Server
	token  *atomic.Pointer[string]
}

func newAdminHandler(server *christmasd.Server, token *atomic.Pointer[string]) *adminHandler {
	h := &adminHandler{
		Mux:    chi.NewRouter(),
		server: server,
		token:  token,
	}

	h.Use(hrt.Use(hrt.Opts{
		Encoder: hrt.CombinedEncoder{
			Encoder: hrt.JSONEncoder, // TODO: swap with text encoder
			Decoder: hrt.URLDecoder,
		},
		ErrorWriter: hrt.TextErrorWriter,
	}))

	h.Patch("/token", h.patchConfig)
	h.Post("/token/randomize", h.randomizeToken)
	h.Post("/kick-all", hrt.Wrap(h.kickAll))

	return h
}

func (h *adminHandler) patchConfig(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	token := string(b)
	h.token.Store(&token)
}

func (h *adminHandler) randomizeToken(w http.ResponseWriter, r *http.Request) {
	uuid, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}

	token := uuid.String()
	h.token.Store(&token)

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(token))
}

type kickAllRequest struct {
	Reason string `query:"reason"`
}

func (h *adminHandler) kickAll(ctx context.Context, req kickAllRequest) (hrt.None, error) {
	h.server.KickAllConnections(req.Reason)
	return hrt.Empty, nil
}
