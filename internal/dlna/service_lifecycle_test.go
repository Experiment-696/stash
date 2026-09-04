package dlna

import (
	"errors"
	"net"
	"testing"
	"time"
)

type lifecycleTestConfig struct{}

func (lifecycleTestConfig) GetDLNAInterfaces() []string          { return nil }
func (lifecycleTestConfig) GetDLNAServerName() string            { return "lifecycle-test" }
func (lifecycleTestConfig) GetDLNADefaultIPWhitelist() []string  { return nil }
func (lifecycleTestConfig) GetVideoSortOrder() string            { return "" }
func (lifecycleTestConfig) GetDLNAPortAsString() string          { return "127.0.0.1:0" }
func (lifecycleTestConfig) GetDLNAActivityTrackingEnabled() bool { return false }

func TestServiceStopWaitsForAsynchronousServeInitialization(t *testing.T) {
	for iteration := 0; iteration < 50; iteration++ {
		initializing := make(chan struct{})
		continueInitialization := make(chan struct{})
		service := NewService(Repository{}, lifecycleTestConfig{}, nil, nil, 0)
		service.activityTracker = nil
		service.serveInitHook = func() {
			close(initializing)
			<-continueInitialization
		}

		if err := service.Start(nil); err != nil {
			t.Fatalf("iteration %d start: %v", iteration, err)
		}
		<-initializing
		address := service.server.HTTPConn.Addr().String()

		stopped := make(chan struct{})
		go func() {
			service.Stop(nil)
			close(stopped)
		}()
		<-service.server.closed
		select {
		case <-stopped:
			t.Fatalf("iteration %d Stop returned before Serve initialization exited", iteration)
		default:
		}

		close(continueInitialization)
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d Stop deadlocked", iteration)
		}
		select {
		case <-service.server.serveDone:
		default:
			t.Fatalf("iteration %d Serve goroutine leaked", iteration)
		}
		if service.IsRunning() {
			t.Fatalf("iteration %d service remained running", iteration)
		}
		if connection, err := net.DialTimeout("tcp", address, 20*time.Millisecond); err == nil {
			connection.Close()
			t.Fatalf("iteration %d listener remained reachable", iteration)
		}

		// Stop and Server.Close are both idempotent after the lifecycle ends.
		service.Stop(nil)
		if err := service.server.Close(); err != nil && !isClosedNetworkError(err) {
			t.Fatalf("iteration %d repeated close: %v", iteration, err)
		}
	}
}

func isClosedNetworkError(err error) bool {
	return errors.Is(err, net.ErrClosed)
}
