package docker

import (
	"aube/pkg/controlplane"
	"log"
	"os"
	"path"

	uuid2 "github.com/google/uuid"
	"github.com/moby/moby/client"
)

const (
	TmpDir           = "./tmp"
	containerTimeout = 1
)

type handler struct {
	name     string
	client   *client.Client
	filePath string
}

type Backend struct {
	client *client.Client
}

func New() *Backend {
	client, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("error creating docker client: %v", err)
		return nil
	}

	return &Backend{
		client: client,
	}
}

// Create creates a new Function based on parameters given by the control plane service.
// name => function name
// fileDir => Directory in which the function is stored
func (b *Backend) Create(name string, filedir string) (controlplane.Handler, error) {
	// create a unique function name
	uuid, err := uuid2.NewRandom()
	if err != nil {
		log.Printf("not able to create new random uuid (this error should not occure)")
		return nil, err
	}

	functionName := name + "-" + uuid.String()
	log.Printf("creating function with name: %s", functionName)

	h := &handler{
		name:   name,
		client: b.client,
	}

	// make a folder for a function
	// mkdir <folder>
	h.filePath = path.Join(TmpDir, functionName)
	err = os.Mkdir(h.filePath, 0777)
	if err != nil {
		log.Printf("creating dir for function: %s failed with error: %v", functionName, err)
		return nil, err
	}

	// copy Docker stuff into the folder
	// cp runtime/python/* <folder>

}
