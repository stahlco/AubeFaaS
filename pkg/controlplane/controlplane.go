package controlplane

import (
	"io"
	"log"
)

type ControlPlane struct {
	functionHandlers []string
}

// Handler is a 'generic' interface for all different Backend (only have Docker for now)
type Handler interface {
	IPs() []string
	Start() error
	Destory() error
	Logs() (io.Reader, error)
}

func (cp *ControlPlane) Stop() error {
	log.Printf("Not implemented yet...")
	return nil
}
