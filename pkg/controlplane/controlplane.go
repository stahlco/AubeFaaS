package controlplane

import "io"

type ControlPlane struct {
}

// Handler is a 'generic' interface for all different Backend (only have Docker for now)
type Handler interface {
	IPs() []string
	Start() error
	Destory() error
	Logs() (io.Reader, error)
}
