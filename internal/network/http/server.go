package peerhttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type ServerHandler struct {
	caller port.PeerCaller
}

func NewServerHandler(caller port.PeerCaller) (*ServerHandler, error) {
	if caller == nil {
		return nil, errors.New("invalid peer caller")
	}
	return &ServerHandler{caller: caller}, nil
}

func (h *ServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != RPCPath || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var req frame
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxFrameBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || decoder.Decode(&struct{}{}) != io.EOF || len(req.Payload) > maxFrameBytes {
		http.Error(w, "invalid frame", http.StatusBadRequest)
		return
	}

	rpcReq := domain.PeerRPCRequest{
		RequestID:  req.RequestID,
		PeerID:     req.PeerID,
		Capability: req.Capability,
		Payload:    req.Payload,
	}

	rpcResp, err := h.caller.Call(r.Context(), rpcReq)
	if err != nil {
		http.Error(w, "rpc failed", http.StatusInternalServerError)
		return
	}

	resp := frame{
		RequestID: rpcResp.RequestID,
		PeerID:    rpcResp.PeerID,
		Payload:   rpcResp.Payload,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return
	}
}
