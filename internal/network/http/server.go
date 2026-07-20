package peerhttp

import (
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type ServerHandler struct {
	caller       port.PeerCaller
	identityFrom func(*x509.Certificate) (string, error)
}

func NewServerHandler(caller port.PeerCaller) (*ServerHandler, error) {
	return NewAuthenticatedServerHandler(caller, PeerIDFromCertificate)
}

func NewAuthenticatedServerHandler(caller port.PeerCaller, identityFrom func(*x509.Certificate) (string, error)) (*ServerHandler, error) {
	if caller == nil {
		return nil, errors.New("invalid peer caller")
	}
	if identityFrom == nil {
		return nil, errors.New("invalid peer identity verifier")
	}
	return &ServerHandler{caller: caller, identityFrom: identityFrom}, nil
}

func (h *ServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != RPCPath || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	callerID, err := authenticatedPeerID(r, h.identityFrom)
	if err != nil {
		http.Error(w, "unauthenticated peer", http.StatusUnauthorized)
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
		CallerID:   callerID,
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

func authenticatedPeerID(r *http.Request, identityFrom func(*x509.Certificate) (string, error)) (string, error) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.VerifiedChains[0]) == 0 {
		return "", errors.New("missing verified client certificate")
	}
	return identityFrom(r.TLS.VerifiedChains[0][0])
}

// PeerIDFromCertificate requires exactly one URI SAN using the stable
// spiffe://motor-autonomo/peer/<id> namespace. CommonName is deliberately
// ignored because it is not a scoped, unambiguous peer identity.
func PeerIDFromCertificate(cert *x509.Certificate) (string, error) {
	if cert == nil || len(cert.URIs) != 1 {
		return "", errors.New("peer certificate must contain exactly one URI SAN")
	}
	uri := cert.URIs[0]
	const prefix = "/peer/"
	if uri.Scheme != "spiffe" || uri.Host != "motor-autonomo" || len(uri.Path) <= len(prefix) || uri.Path[:len(prefix)] != prefix || uri.RawQuery != "" || uri.Fragment != "" {
		return "", errors.New("invalid peer URI SAN")
	}
	id := uri.Path[len(prefix):]
	if !validPeerID(id) {
		return "", errors.New("invalid peer ID")
	}
	return id, nil
}

func validPeerID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for i := range id {
		c := id[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}
