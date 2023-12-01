package main

import (
	"context"

	"dev.acmcsuf.com/christmasd"
	"github.com/go-chi/chi/v5"
	"libdb.so/hrt"
)

type adminHandler struct {
	*chi.Mux
	server *christmasd.Server
}

func newAdminHandler(server *christmasd.Server) *adminHandler {
	h := &adminHandler{
		Mux:    chi.NewRouter(),
		server: server,
	}

	h.Use(hrt.Use(hrt.Opts{
		Encoder: hrt.CombinedEncoder{
			Encoder: hrt.JSONEncoder, // TODO: swap with text encoder
			Decoder: hrt.URLDecoder,
		},
		ErrorWriter: hrt.TextErrorWriter,
	}))

	h.Patch("/config", hrt.Wrap(h.patchConfig))
	h.Post("/kick-all", hrt.Wrap(h.kickAll))

	return h
}

type patchConfigRequest struct {
	Secret string `query:"secret"`
}

func (h *adminHandler) patchConfig(ctx context.Context, req patchConfigRequest) (hrt.None, error) {
	h.server.SetConfig(christmasd.Config{Secret: req.Secret})
	return hrt.Empty, nil
}

type kickAllRequest struct {
	Reason string `query:"reason"`
}

func (h *adminHandler) kickAll(ctx context.Context, req kickAllRequest) (hrt.None, error) {
	h.server.KickAllConnections(req.Reason)
	return hrt.Empty, nil
}
