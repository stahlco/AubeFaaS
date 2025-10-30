package main

import (
	"aube/pkg/controlplane"
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"

	"github.com/google/uuid"
)

const (
	ConfigPort          = 8080
	RProxyConfigPort    = 8081
	RProxyListenAddress = "localhost"
)

// For what do I need the Control Plane?
// - Managing the creation of a Backend -> So not a static function
// - Managing the deletion of a Backenc
// - Communicating with the RProxy

type server struct {
	cp *controlplane.ControlPlane
}

func main() {
	log.SetPrefix("cp: ")
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	log.Printf("controlplane started")

	// start the proxy
	rproxyArgs := []string{fmt.Sprintf("%s:%d", RProxyListenAddress, RProxyConfigPort)}
	rproxyArgs = append(rproxyArgs, fmt.Sprintf("%s://%s:%d", "ws", RProxyListenAddress, 8083))

	log.Println("rproxy args: ", rproxyArgs)

	id := uuid.New().String()

	rProxyDir := path.Join(os.TempDir(), id)

	err := os.MkdirAll(rProxyDir, 0755)
	if err != nil {
		log.Printf("creating rproxyDir failed with error: %v", err)
		os.Exit(1)
	}

	rProxyPath := path.Join(rProxyDir, "rproxy.bin")

	err = os.WriteFile(rProxyPath, RProxyBin, 0755)
	if err != nil {
		log.Printf("writing rproxy-darwin-arm64.bin in rporxy.bin into the rproxyDir failed with error: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(rProxyDir)

	c := exec.Command(rProxyPath, rproxyArgs...)

	// Pipe that will be connected to the command's stdout when the command start
	stdout, err := c.StdoutPipe()
	if err != nil {
		log.Printf("Getting the stdout pipe for starting rporxy failed with err: %v", err)
		os.Exit(1)
	}

	stderr, err := c.StderrPipe()
	if err != nil {
		log.Printf("Getting the stderr pipe for starting rproxy failed with err: %v", err)
		os.Exit(1)
	}

	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			fmt.Printf(s.Text())
		}
	}()

	go func() {
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			fmt.Printf(s.Text())
		}
	}()

	err = c.Start()
	if err != nil {
		log.Printf("starting the command to start the rproxy failed with error: %v", err)
		os.Exit(1)
	}

	rproxy := c.Process

	log.Printf("started rproxy")

	// Creating a ControlPlane instance

	cp := &controlplane.ControlPlane{}

	s := &server{
		cp: cp,
	}

	//create handlers
	r := http.NewServeMux()
	r.HandleFunc("/upload", s.uploadHandler)
	r.HandleFunc("/delete", s.deleteHandler)

	// Shutdown-Hook
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig

		log.Printf("shutting down (received interrupt)")

		log.Printf("stopping rproxy")
		err := rproxy.Kill()
		if err != nil {
			log.Printf("killing rproxy failed with err: %v", err)
		}
		err = s.cp.Stop()
		if err != nil {
			log.Printf("stopping controlplane failed with error: %v", err)
		}
		os.Exit(0)
	}()
}

func (s *server) uploadHandler(w http.ResponseWriter, req *http.Request) {
	// Will add functionality later
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *server) deleteHandler(w http.ResponseWriter, req *http.Request) {
	// Will add functionality later
	w.WriteHeader(http.StatusNotImplemented)
}
