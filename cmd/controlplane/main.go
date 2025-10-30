package main

import "log"

const (
	ConfigPort          = 8080
	RProxyConfigPort    = 8081
	RProxyListenAddress = ""
)

// For what do I need the Control Plane?
// - Managing the creation of a Backend -> So not a static function
// - Managing the deletion of a Backenc
// - Communicating with the RProxy

type controlplane struct {
}

func main() {
	log.SetPrefix("cp: ")
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	log.Printf("controlplane started")
}
