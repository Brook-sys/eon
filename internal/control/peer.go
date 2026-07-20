package control

import (
	"context"
	"errors"
	"fmt"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// AddPeerCommand registers a new peer in the local P2P registry.
type AddPeerCommand struct {
	ID         string
	Host       string
	Port       int
	PublicKey  []byte
}

func (c AddPeerCommand) Validate() error {
	if c.ID == "" {
		return errors.New("peer id is required")
	}
	if c.Host == "" {
		return errors.New("peer host is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("peer port must be between 1 and 65535")
	}
	return nil
}

// PeerManager allows the control API to mutate the local network registry.
type PeerManager struct {
	Registry port.PeerRegistry
}

// AddPeer handles the AddPeerCommand.
func (pm *PeerManager) AddPeer(ctx context.Context, cmd AddPeerCommand) error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("invalid add peer command: %w", err)
	}

	if pm.Registry == nil {
		return errors.New("peer networking is not enabled on this node")
	}

	record := domain.PeerRecord{
		Identity: domain.NodeIdentity{
			ID:        cmd.ID,
			PublicKey: cmd.PublicKey,
		},
		Address: domain.PeerAddress{
			Host: cmd.Host,
			Port: cmd.Port,
		},
		Capabilities: []string{"rpc"}, // Default capability
	}

	if err := pm.Registry.Register(ctx, record); err != nil {
		return fmt.Errorf("register peer: %w", err)
	}

	return nil
}
