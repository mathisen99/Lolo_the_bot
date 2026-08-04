package irc

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"gopkg.in/irc.v4"
)

func TestRunMarksFailedConnectionDisconnected(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	t.Cleanup(func() {
		_ = clientSide.Close()
		_ = serverSide.Close()
	})

	wrapped := irc.NewClient(clientSide, irc.ClientConfig{
		Nick: "Lolo",
		User: "lolo",
		Name: "Lolo IRC Bot",
	})

	client := NewClient(nil, noopLogger{})
	client.mu.Lock()
	client.conn = wrapped
	client.rawConn = clientSide
	client.connected = true
	client.ctx, client.cancel = context.WithCancel(context.Background())
	connectionContext := client.ctx
	client.mu.Unlock()

	// Drain the registration writes, then emulate a remote socket reset by
	// closing the server side of the pipe.
	go func() {
		buffer := make([]byte, 1024)
		_, _ = serverSide.Read(buffer)
		_ = serverSide.Close()
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run()
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("Run returned nil after the connection closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the connection closed")
	}

	if client.IsConnected() {
		t.Fatal("client remained connected after its event loop stopped")
	}

	client.mu.RLock()
	if client.conn != nil {
		t.Error("failed IRC client was not cleared")
	}
	if client.rawConn != nil {
		t.Error("failed raw connection was not cleared")
	}
	client.mu.RUnlock()

	select {
	case <-connectionContext.Done():
	case <-time.After(time.Second):
		t.Fatal("connection context was not cancelled")
	}
}

func TestMarkConnectionLostDoesNotClearReplacement(t *testing.T) {
	client := NewClient(nil, noopLogger{})
	oldConn := irc.NewClient(&errorReadWriteCloser{}, irc.ClientConfig{})
	replacement := irc.NewClient(&errorReadWriteCloser{}, irc.ClientConfig{})

	client.mu.Lock()
	client.conn = replacement
	client.rawConn = &errorReadWriteCloser{}
	client.connected = true
	client.mu.Unlock()

	client.markConnectionLost(oldConn)

	if !client.IsConnected() {
		t.Fatal("late failure from old event loop cleared the replacement connection")
	}
	client.mu.RLock()
	defer client.mu.RUnlock()
	if client.conn != replacement {
		t.Fatal("replacement IRC client was cleared")
	}
}

func TestRunWaitsForReplacementAfterConnectionContextIsCancelled(t *testing.T) {
	client := NewClient(nil, noopLogger{})
	client.cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run()
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before a replacement connection was available: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	replacement := irc.NewClient(&errorReadWriteCloser{}, irc.ClientConfig{
		Nick: "Lolo",
		User: "lolo",
		Name: "Lolo IRC Bot",
	})
	client.mu.Lock()
	client.conn = replacement
	client.rawConn = &errorReadWriteCloser{}
	client.connected = true
	client.mu.Unlock()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("Run returned nil for the failing replacement connection")
		}
		if err.Error() == "context cancelled while waiting for connection" {
			t.Fatal("Run reused the cancelled context instead of waiting for reconnection")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not start the replacement connection")
	}
}

type errorReadWriteCloser struct{}

func (*errorReadWriteCloser) Read([]byte) (int, error)  { return 0, io.EOF }
func (*errorReadWriteCloser) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func (*errorReadWriteCloser) Close() error              { return nil }
