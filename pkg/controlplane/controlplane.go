package controlplane

import (
	"aube/pkg/util"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"slices"
	"sync"

	uuid2 "github.com/google/uuid"
)

const (
	TmpDir = "./tmp"
)

type ControlPlane struct {
	id                 string
	FunctionHandlers   map[string]Handler
	functionHandlerMtx sync.Mutex
	rproxyListenAddr   string
	rproxyConfigPort   int
	backend            Backend
}

// Backend has only the Docker implementation
type Backend interface {
	// Create creates a function in the Backend -> used by the upload script
	Create(name string, filedir string, initThreads int, maxThreads int) (Handler, error)
	Stop() error
}

// Handler is a 'generic' interface for all different Backend (only have Docker for now)
type Handler interface {
	IPs() []string
	// StartContainer will be triggered after Add was invoked successfully
	StartContainer(name string) error
	// Start will be triggered right after creation of the initial containers
	Start() error
	Add() (string, error)
	Delete(name string) error
	Destroy() error
	Logs() (io.Reader, error)
}

func New(id string, rproxyListenAddr string, rproxyConfigPort int, backend Backend) *ControlPlane {
	return &ControlPlane{
		id:                 id,
		FunctionHandlers:   make(map[string]Handler),
		functionHandlerMtx: sync.Mutex{},
		rproxyListenAddr:   rproxyListenAddr,
		rproxyConfigPort:   rproxyConfigPort,
		backend:            backend,
	}
}

func (cp *ControlPlane) Stop() error {
	log.Printf("not implemented yet...")
	return nil
}

func (cp *ControlPlane) createFunction(name string, fnzip []byte, subfolderPath string) (string, error) {
	log.Printf("createFunction received following args: %s, %d", name, len(fnzip))
	uuid, err := uuid2.NewRandom()
	if err != nil {
		log.Printf("not able to create uuid with error: %v", err)
		return "", err
	}

	log.Printf("creating function %s, with uuid: %s", name, uuid.String())

	p := path.Join(TmpDir, uuid.String())
	err = os.MkdirAll(p, 0777)
	if err != nil {
		log.Printf("not able to create directory: %s with err: %v", p, err)
		return "", err
	}

	zipPath := path.Join(TmpDir, uuid.String()+".zip")
	err = os.WriteFile(zipPath, fnzip, 0777)
	if err != nil {
		return "", err
	}

	err = util.Unzip(zipPath, p)
	if err != nil {
		log.Printf("not able to unzip function zip: %v", err)
		return "", err
	}

	// Remove all Temp Directories that are not longer needed
	defer func() {
		err = os.RemoveAll(p)
		if err != nil {
			log.Printf("error removing folder %s: %v", p, err)
		}

		err = os.Remove(zipPath)
		if err != nil {
			log.Printf("error removing zip %s: %v", p, err)
		}

		log.Println("removed folder")
		log.Println("removed zip")
	}()

	if subfolderPath != "" {
		p = path.Join(p, subfolderPath)
	}

	// What are we doing if the function already exists? -> Deploy a new one

	var oldHandler Handler
	if existingHandler, ok := cp.FunctionHandlers[name]; ok {
		oldHandler = existingHandler
	}

	cp.functionHandlerMtx.Lock()
	defer cp.functionHandlerMtx.Unlock()

	// Now just Mock stuff, need to switch the upload script!
	// Hier kriegen wir einen Handler zurück!
	// TODO
	fh, err := cp.backend.Create(name, p, 1, 10)
	if err != nil {
		log.Printf("creating the function handler failed with err: %v", err)
		return "", err
	}

	cp.FunctionHandlers[name] = fh

	err = cp.FunctionHandlers[name].Start()
	if err != nil {
		return "", err
	}

	// Register function at the RProxy
	d := struct {
		FunctionName string   `json:"name"`
		FunctionIPs  []string `json:"ips"`
	}{
		FunctionName: name,
		FunctionIPs:  fh.IPs(),
	}

	b, err := json.Marshal(d)
	if err != nil {
		log.Printf("failed to marshall the payload to register the function at the proxy: %v", err)
		return "", err
	}

	log.Printf("telling rproxy about new function %s, with ips %v, : %+v", name, fh.IPs(), d)

	resp, err := http.Post(fmt.Sprintf("http://%s:%d", cp.rproxyListenAddr, cp.rproxyConfigPort), "application/json", bytes.NewBuffer(b))
	if err != nil && errors.Is(err, io.EOF) {
		log.Printf("error telling rproxy about the new function %s: %v", name, err)
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		// could add any form of retries, but not important for now
		log.Printf("received a not expected status code from rproxy: %d", resp.StatusCode)
		return "", fmt.Errorf("rproxy returned status code %d", resp.StatusCode)
	}

	if oldHandler != nil {
		err = oldHandler.Destroy()
		if err != nil {
			return "", err
		}
	}

	return name, nil
}

func (cp *ControlPlane) Upload(name string, zippedString string) (string, error) {

	//base64 decode zip
	zip, err := base64.StdEncoding.DecodeString(zippedString)
	if err != nil {
		log.Printf("not able to base64 decode the zipped with err: %v", err)
		return "", err
	}

	functionName, err := cp.createFunction(name, zip, "")
	if err != nil {
		log.Printf("not able to create function: %s with error: %v", name, err)
		return "", err
	}

	r := fmt.Sprintf("http://%s:%d/%s\n", cp.rproxyListenAddr, 8093, functionName)

	return r, nil
}

// Scale Wie kriegen wir die IPs wieder zum Proxy?
// returns: a list of IPs which have been added
func (cp *ControlPlane) Scale(name string, amount int) ([]string, error) {
	var handler Handler
	if existingHandler, ok := cp.FunctionHandlers[name]; ok {
		handler = existingHandler
	} else {
		return nil, http.ErrMissingFile // Represents
	}

	// If we have the handler, what do we want to do!
	// Create a new Container! -> Start the container -> and return IPs to the RProxy so it can add them!

	// Before we are creating a new function -> get IPs than find differences

	var ips []string

	for i := 0; i < amount; i++ {
		prev := handler.IPs()
		containerName, err := handler.Add()

		if err != nil {
			return nil, err
		}

		err = handler.StartContainer(containerName)
		if err != nil {
			return nil, err
		}

		curr := handler.IPs()

		slices.Sort(prev)
		slices.Sort(curr)

		for i := 0; i < len(prev); i++ {
			if prev[i] != curr[i] {
				containerName = curr[i]
			}
		}
		ip := curr[len(curr)-1]

		ips = append(ips, ip)
	}

	return ips, nil
}
