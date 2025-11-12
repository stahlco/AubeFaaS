package rproxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"slices"
	"sync"
)

const (
	UrlPrefix    = "ws"
	FunctionPort = 8000
)

// Function will be added soon -> Multi-Tenancy
type Function struct {
	name string
	// uniqueContainerName -> IP
	//maybe add a Timestamp in the Future and an async clean-up
	freeIPs []string
	usedIPs []string
	hl      sync.RWMutex
}

func NewFunction(name string, ips []string) *Function {
	return &Function{
		name:    name,
		freeIPs: ips,
		usedIPs: make([]string, 0),
		hl:      sync.RWMutex{},
	}
}

func (f *Function) useContainer(containerIP string) error {

	if !slices.Contains(f.freeIPs, containerIP) {
		return fmt.Errorf("%s not found in free container list", containerIP)
	}

	if slices.Contains(f.usedIPs, containerIP) {
		return fmt.Errorf("%s is not in free containers but used containers", containerIP)
	}

	f.hl.Lock()

	remove(f.freeIPs, containerIP)
	f.usedIPs = append(f.usedIPs, containerIP)

	f.hl.Unlock()

	return nil
}

func (f *Function) freeContainer(containerIP string) error {

	if !slices.Contains(f.usedIPs, containerIP) {
		return fmt.Errorf("%s not found in used containers", containerIP)
	}

	if slices.Contains(f.freeIPs, containerIP) {
		return fmt.Errorf("%s is not in used containers but in free containers", containerIP)
	}

	f.hl.Lock()

	remove(f.usedIPs, containerIP)
	f.freeIPs = append(f.freeIPs, containerIP)

	f.hl.Unlock()

	return nil
}

func (f *Function) getContainer() (string, error) {

	if len(f.freeIPs) == 0 {
		err := f.scaleFunction()
		if err != nil {
			return "", err
		}
	}

	containerIP := f.freeIPs[rand.Intn(len(f.freeIPs))]

	// Block the container straight up
	err := f.useContainer(containerIP)
	if err != nil {
		return "", err
	}

	// Build the proper function URL
	url := fmt.Sprintf("%s://%s:%d", UrlPrefix, containerIP, FunctionPort)

	return url, nil
}

func (f *Function) scaleFunction() error {

	b := new(bytes.Buffer)
	d := struct {
		FunctionName string `json:"name"`
		Amount       int    `json:"amount"`
	}{
		FunctionName: f.name,
		Amount:       1, //Optimization later
	}

	err := json.NewEncoder(b).Encode(d)
	if err != nil {
		return err
	}

	resp, err := http.Post("http://localhost:8090", "application/json", b)
	if err != nil || resp == nil {
		return fmt.Errorf("resp nil or err: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("not able to scale the function received http status code: %v", resp.StatusCode)
	}

	r := struct {
		NewIPs []string `json:"ips"`
	}{}

	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		return err
	}

	// Add the new IPs to the freeIPs
	f.freeIPs = append(f.freeIPs, r.NewIPs...)

	return nil
}

func remove[T comparable](l []T, item T) []T {
	for i, other := range l {
		if other == item {
			return append(l[:i], l[i+1:]...)
		}
	}
	return l
}
