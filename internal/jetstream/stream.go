package jetstream

import (
	"strings"
	"time"

	nats "github.com/nats-io/nats.go"
)

const (
	StreamName    = "SIDEKICK"
	SubjectPrefix = "sidekick.req."
)

func EnsureStream(js nats.JetStreamContext) error {
	_, err := js.AddStream(&nats.StreamConfig{
		Name:      StreamName,
		Subjects:  []string{"sidekick.>"},
		Storage:   nats.FileStorage,
		MaxAge:    24 * time.Hour,
		Retention: nats.WorkQueuePolicy,
	})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}
	return nil
}

func ChunkSubject(requestID string) string {
	return SubjectPrefix + requestID
}

func DoneSubject(requestID string) string {
	return SubjectPrefix + requestID + ".done"
}
