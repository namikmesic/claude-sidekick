package jetstream

import (
	"fmt"
	"time"

	server "github.com/nats-io/nats-server/v2/server"
	nats "github.com/nats-io/nats.go"
)

type Server struct{ ns *server.Server }

func NewServer(storeDir string) (*Server, error) {
	ns, err := server.NewServer(&server.Options{
		DontListen: true,
		JetStream:  true,
		StoreDir:   storeDir,
	})
	if err != nil {
		return nil, err
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		return nil, fmt.Errorf("NATS server not ready")
	}
	return &Server{ns: ns}, nil
}

func (s *Server) Connect() (*nats.Conn, error) {
	return nats.Connect(s.ns.ClientURL(), nats.InProcessServer(s.ns))
}

func (s *Server) Shutdown() {
	s.ns.Shutdown()
	s.ns.WaitForShutdown()
}
