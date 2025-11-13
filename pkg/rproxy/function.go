package rproxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
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

	log.Printf("successfully passed checks")
	f.hl.Lock()

	log.Printf("BEFORE: container lists of function: FREE: %v, USED: %v", f.freeIPs, f.usedIPs)

	f.freeIPs = remove(f.freeIPs, containerIP)
	f.usedIPs = append(f.usedIPs, containerIP)

	log.Printf("AFTER: container lists of function: FREE: %v, USED: %v", f.freeIPs, f.usedIPs)

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

	f.usedIPs = remove(f.usedIPs, containerIP)
	f.freeIPs = append(f.freeIPs, containerIP)

	f.hl.Unlock()

	return nil
}

func (f *Function) getContainer() (string, error) {
	log.Printf("trying to get a free container: %v", f.freeIPs)

	if len(f.freeIPs) == 0 {
		log.Printf("now trying to scale the function")
		err := f.scaleFunction()
		if err != nil {
			log.Printf("error scaling the function")
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

	log.Printf("now sending a http.Post to \"http://localhost:8090/scale\"")

	resp, err := http.Post("http://localhost:8090/scale", "application/json", b)
	if err != nil || resp == nil {
		log.Printf("error in response")
		return fmt.Errorf("resp nil or err: %v", err)
	}

	log.Printf("received this response from cp: %v", resp.StatusCode)

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

	log.Printf("decoded response into r: %v", r)

	log.Printf("free_ips before update should be: [], is: %v", f.freeIPs)
	// Add the new IPs to the freeIPs
	f.freeIPs = append(f.freeIPs, r.NewIPs...)

	log.Printf("free_ips before update should be not: [], is: %v", f.freeIPs)

	return nil
}

func remove[T comparable](list []T, item T) []T {
	temp := list[:0]
	for _, listItem := range list {
		if listItem != item {
			temp = append(temp, listItem)
		}
	}
	// Optimize Performance -> Must be garbage collected anyway
	clear(list[len(temp):])
	return temp
}
