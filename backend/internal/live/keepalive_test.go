package live

import (
	"context"
	"errors"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

func TestRunPingLoopClosesConnectionOnPingTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := &blockingPinger{
		pingStarted: make(chan struct{}, 1),
		closed:      make(chan string, 1),
	}

	go runPingLoop(ctx, conn, time.Millisecond, time.Millisecond)

	select {
	case <-conn.pingStarted:
	case <-time.After(time.Second):
		t.Fatal("ping was not attempted")
	}

	select {
	case reason := <-conn.closed:
		if reason != ErrorPingTimeout {
			t.Fatalf("close reason = %q, want %q", reason, ErrorPingTimeout)
		}
	case <-time.After(time.Second):
		t.Fatal("connection was not closed after ping timeout")
	}
}

type blockingPinger struct {
	pingStarted chan struct{}
	closed      chan string
}

func (p *blockingPinger) Ping(ctx context.Context) error {
	select {
	case p.pingStarted <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func (p *blockingPinger) Close(_ websocket.StatusCode, reason string) error {
	select {
	case p.closed <- reason:
	default:
		return errors.New("connection already closed")
	}
	return nil
}
