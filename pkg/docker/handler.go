package docker

import (
	"aube/pkg/controlplane"
	"aube/pkg/util"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"slices"
	"sync"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	uuid2 "github.com/google/uuid"
	"github.com/moby/go-archive"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const (
	TmpDir = "./tmp"
)

type DockerBackend struct {
	id     string
	client *client.Client
}

// Each dockerHandler represents a single function with n containers
type dockerHandler struct {
	name        string
	uniqueName  string // Determines Image and Network as well
	initThreads int
	maxThreads  int
	filePath    string
	// Docker specific stuff -> needed to create or remove containers
	client          *client.Client
	containers      []string
	containerIPs    []string
	network         string
	containerConfig *container.Config
	hostConfig      *container.HostConfig
}

func New(aubeFaaSID string) (*DockerBackend, error) {
	id := "AubeFaaS-" + aubeFaaSID

	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return &DockerBackend{
		id:     id,
		client: c,
	}, nil
}

// Create creates a new function with function-name: name in the file-directory
// filedir must include ./fn.py and ./requirements -> this will be loaded into to the container
// filedir would be: ./test/fn
func (d DockerBackend) Create(name string, filedir string, initThreads int, maxThreads int) (controlplane.Handler, error) {

	// Create a new unique function name
	uuid, err := uuid2.NewRandom()
	if err != nil {
		return nil, err
	}

	uniqueName := name + "-" + uuid.String()

	handler := &dockerHandler{
		name:         name,
		uniqueName:   uniqueName,
		client:       d.client,
		initThreads:  initThreads,
		maxThreads:   maxThreads,
		containers:   make([]string, 0, maxThreads),
		containerIPs: make([]string, 0, maxThreads),
	}

	// Copy the Docker-Runtime into a folder
	// cp runtimes/python/* ./tmp/<uniqueName>
	handler.filePath = path.Join(TmpDir, handler.uniqueName) // mkdir <folder>
	log.Printf("Create Folder: %s", handler.filePath)

	err = os.MkdirAll(handler.filePath, 0777)
	if err != nil {
		return nil, err
	}

	err = util.CopyDirFromEmbed(runtimes, path.Join(runtimesDir, "python"), handler.filePath)
	if err != nil {
		log.Printf("copying embed filesystem (python-runtime) into function failed with err: %v", err)
		return nil, err
	}

	// Copy function-code into the runtime
	// cp <filedir> <folder>/fn

	functionFilePath := path.Join(handler.filePath, "fn")
	log.Printf("Create functionFilePath: %s", functionFilePath)

	err = os.MkdirAll(functionFilePath, 0777)
	if err != nil {
		log.Printf("creating folder for function failed with err: %v", err)
		return nil, err
	}

	err = util.CopyAll(filedir, functionFilePath)
	if err != nil {
		log.Printf("copying function code into fn-folder failed with err: %v", err)
		return nil, err
	}

	tar, err := archive.TarWithOptions(handler.filePath, &archive.TarOptions{})
	if err != nil {
		return nil, err
	}

	log.Printf("Created tar: %+v", tar)

	imageBuildOpts := client.ImageBuildOptions{
		Tags:       []string{handler.uniqueName}, // needed for identifying the image
		Dockerfile: "Dockerfile",
		Remove:     true,
		Labels: map[string]string{
			"AubeFaaS-Function": handler.uniqueName,
			"AubeFaaS-ID":       d.id,
		},
	}

	imageResp, err := handler.client.ImageBuild(context.Background(), tar, imageBuildOpts)
	if err != nil {
		log.Printf("Building failed with error: %v", err)
		return nil, err
	}
	defer imageResp.Body.Close()
	// Reading Body from Image Creation
	scanner := bufio.NewScanner(imageResp.Body)
	for scanner.Scan() {
		log.Println(scanner.Text())
	}

	networkOpts := client.NetworkCreateOptions{
		Labels: map[string]string{
			"AubeFaaS-Function": handler.name,
			"AubeFaaS-ID":       d.id,
		},
	}

	nw, err := handler.client.NetworkCreate(context.Background(), handler.uniqueName, networkOpts)
	if err != nil {
		return nil, err
	}

	handler.network = nw.ID

	containerConfig := &container.Config{
		Image: handler.uniqueName,
		Labels: map[string]string{
			"AubeFaaS-Function": handler.uniqueName,
			"AubeFaaS-ID":       d.id,
		},
	}

	handler.containerConfig = containerConfig

	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(handler.uniqueName),
	}

	handler.hostConfig = hostConfig

	err = createContainer(handler, initThreads)
	if err != nil {
		return nil, err
	}

	return handler, nil
}

func createContainer(handler *dockerHandler, amount int) error {

	if curr := len(handler.containers); (curr + amount) > handler.maxThreads {
		amount = handler.maxThreads - curr
		log.Printf("Not able to create more than %d container because it would exceed the upper resource bound", amount)
	}

	for i := 0; i < amount; i++ {
		idx := len(handler.containers)

		c, err := handler.client.ContainerCreate(
			context.Background(),
			handler.containerConfig,
			handler.hostConfig,
			nil,
			nil,
			handler.uniqueName+fmt.Sprintf("-%d", idx),
		)
		if err != nil {
			return err
		}
		handler.containers = append(handler.containers, c.ID)
	}

	return nil
}

// Add allows that we can scale-out, this function adds a single new container.
// So for adding several instances Add must be called the desired amount of times.
func (handler *dockerHandler) Add() (string, error) {

	prev := handler.containers

	err := createContainer(handler, 1)
	if err != nil {
		return "", err
	}

	curr := handler.containers

	slices.Sort(prev)
	slices.Sort(curr)

	var containerName string

	for i := 0; i < len(prev); i++ {
		if prev[i] != curr[i] {
			containerName = curr[i]
		}
	}
	containerName = curr[len(curr)-1]

	return containerName, err
}

func (handler *dockerHandler) StartContainer(name string) error {
	log.Printf("starting container with name: %s", name)
	wg := sync.WaitGroup{}

	wg.Add(1)

	errChan := make(chan error, 1)
	go func(name string) {
		defer wg.Done()
		err := handler.client.ContainerStart(context.Background(), name, client.ContainerStartOptions{})
		if err != nil {
			errChan <- err
			log.Printf("Not able to start container")
			return
		}
		// this prevents blocking
		errChan <- nil
		log.Printf("started container with name: %s", name)
	}(name)

	wg.Wait()

	err := <-errChan
	if err != nil {
		return err
	}

	log.Printf("inspecting container: %s", name)
	// get container IP
	insp, err := handler.client.ContainerInspect(context.Background(), name)
	if err != nil {
		log.Printf("not able to inspect container %s with err: %v", name, err)
		return err
	}

	log.Printf("networks of inspection of %s: %+v", name, insp.NetworkSettings.Networks)
	ip := insp.NetworkSettings.Networks[handler.uniqueName].IPAddress.String()
	log.Printf("inspected following ip: %s to containerIPs: %v", ip, handler.containerIPs)
	handler.containerIPs = append(handler.containerIPs, ip)
	log.Printf("added ip to ips now: %v", handler.containerIPs)

	retries := 3

	for i := 0; i < retries; i++ {
		if i == retries-1 {
			log.Printf("Container not able to start with 3 retries, logs:")
			logs, err := handler.getContainerLogs(name)
			if err != nil {
				log.Printf("error occured printing logs, aborting: %v", err)
				return err
			}
			log.Print(logs)
		}

		time.Sleep(1)

		// timeout of 3 seconds
		client := http.Client{
			Timeout: 3 * time.Second,
		}

		resp, err := client.Get("http://" + ip + ":8080/health")
		if err != nil {
			log.Println(err)
			log.Println("retrying in 1 second")
			time.Sleep(1 * time.Second)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			log.Println("container", ip, "is ready")
			break
		}
		log.Println("container", ip, "is not ready yet, retrying in 1 second")
		time.Sleep(1 * time.Second)
	}

	return nil
}

func (handler *dockerHandler) Start() error {
	wg := sync.WaitGroup{}

	// Is this important?
	// errChan := make(chan error)
	for _, c := range handler.containers {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()
			err := handler.client.ContainerStart(context.Background(), c, client.ContainerStartOptions{})
			if err != nil {
				log.Printf("Error starting container: %v", err)
				return
			}

			log.Printf("successfully started container: %s", c)
		}(c)
	}

	wg.Wait()

	log.Printf("Successfully started all container")

	// get container IPs
	// docker inspect <container>
	for _, c := range handler.containers {
		insp, err := handler.client.ContainerInspect(context.Background(), c)
		if err != nil {
			log.Printf("inspecting container failed with error")
			return err
		}
		ip := insp.NetworkSettings.Networks[handler.uniqueName].IPAddress.String()
		log.Printf("IP-Address of Container %v is: %s", c, ip)
		handler.containerIPs = append(handler.containerIPs, ip)
	}

	log.Printf("FH: %v, This the list of ips fetched: %v", handler, handler.containerIPs)
	return nil
}

// Delete deletes a specific Container based on the given
func (handler *dockerHandler) Delete(containerIP string) error {
	if !slices.Contains(handler.containerIPs, containerIP) {
		return fmt.Errorf("containerIP: %s does not exist in List of containerIPs", containerIP)
	}

	containerID := ""
	// Now we need to find the container which is represented by the containerIP
	for _, c := range handler.containers {
		inspect, err := handler.client.ContainerInspect(context.Background(), c)
		if err != nil {
			log.Printf("Failed to inspect container with id: %s ", c)
			return err
		}

		for _, net := range inspect.NetworkSettings.Networks {
			if net.IPAddress.String() == containerIP {
				containerID = c
				break
			}
		}

		if containerID != "" {
			break
		}
	}

	err := handler.client.ContainerStop(context.Background(), containerID, client.ContainerStopOptions{})
	if err != nil {
		log.Printf("stopping container %s failed with err: %s, please remove manually", containerID, err)
		return err
	}

	err = handler.client.ContainerRemove(context.Background(), containerID, client.ContainerRemoveOptions{})
	if err != nil {
		log.Printf("removing container %s failed with error: %v, please remove manually", containerID, err)
		return err
	}

	// remove Container from Containers

	for i := 0; i < len(handler.containers); i++ {
		if handler.containers[i] == containerID {
			handler.containers = append(handler.containers[:i], handler.containers[(i+1):]...)
			break
		}
	}

	// remove ContainerIP from ContainerIPs

	for i := 0; i < len(handler.containerIPs); i++ {
		if handler.containerIPs[i] == containerIP {
			handler.containerIPs = append(handler.containerIPs[:i], handler.containerIPs[(i+1):]...)
			break
		}
	}

	return nil
}

func (d DockerBackend) Stop() error {
	return fmt.Errorf("currently not implemented")
}

func (handler *dockerHandler) IPs() []string {
	return handler.containerIPs
}

// Destroy cleans up the complete function, so every container gets shut down
func (handler *dockerHandler) Destroy() error {

	wg := sync.WaitGroup{}
	for _, c := range handler.containers {

		wg.Add(1)
		go func(c string) {
			defer wg.Done()
			err := handler.client.ContainerStop(context.Background(), c, client.ContainerStopOptions{})
			if err != nil {
				log.Printf("not able to stop container %s with err: %v, please remove manually", c, err)
				return
			}

			err = handler.client.ContainerRemove(context.Background(), c, client.ContainerRemoveOptions{})
			if err != nil {
				log.Printf("not able to remove container %s with err: %v, please remove manually", c, err)
				return
			}
		}(c)
	}
	wg.Wait()

	err := handler.client.NetworkRemove(context.Background(), handler.network)
	if err != nil {
		log.Printf("not able to remove network: %s with err: %v, please remove manually", handler.network, err)
	}

	// We need to remove the image
	_, err = handler.client.ImageRemove(context.Background(), handler.uniqueName, client.ImageRemoveOptions{})
	if err != nil {
		log.Printf("not able to remove the image")
		return err
	}

	return nil
}

func (handler *dockerHandler) Logs() (io.Reader, error) {
	return nil, fmt.Errorf("currently not implemented")
}

func (handler *dockerHandler) getContainerLogs(name string) (string, error) {
	logs := ""

	l, err := handler.client.ContainerLogs(
		context.Background(),
		name,
		client.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Timestamps: true,
		})
	if err != nil {
		log.Printf("not able to fetch the container logs")
		return "", err
	}

	var lstdout bytes.Buffer
	var lstderr bytes.Buffer

	_, err = stdcopy.StdCopy(&lstdout, &lstderr, l)
	l.Close()
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(&lstdout)
	for scanner.Scan() {
		logs += fmt.Sprintf("function=%s handler=%s %s\n", handler.name, name, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return logs, err
	}

	scanner = bufio.NewScanner(&lstderr)

	for scanner.Scan() {
		logs += fmt.Sprintf("function=%s handler=%s %s\n", handler.name, name, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return logs, err
	}

	return logs, nil
}
