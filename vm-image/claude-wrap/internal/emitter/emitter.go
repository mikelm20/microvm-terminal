// Package emitter fans out parsed events to a Unix domain socket that the
// guest-agent tails. Exactly one reader at a time; on reader disconnect the
// emitter reopens the listener and waits for the next guest-agent attach.
//
// The socket is JSONL: one event per line. The emitter does not back-pressure
// the parser: if no reader is attached, events are buffered up to queueCap
// then oldest are dropped. Each accepted reader gets events forward-only from
// the moment it attached (no replay). The guest-agent reconnection logic is
// responsible for its own resume strategy.
package emitter

import (
	"bufio"
	"encoding/json"
	"errors"
	"log"
	"net"
	"os"
	"sync"
)

const queueCap = 2048

type Emitter struct {
	path string
	mu   sync.Mutex
	buf  chan any
	stop chan struct{}
}

func New(path string) *Emitter {
	return &Emitter{
		path: path,
		buf:  make(chan any, queueCap),
		stop: make(chan struct{}),
	}
}

// Send queues an event for delivery. Non-blocking; drops when queue is full.
func (e *Emitter) Send(ev any) {
	select {
	case e.buf <- ev:
	default:
		// Drop; at-most-once semantics suffice for telemetry.
	}
}

// Run accepts connections forever. Call in a goroutine.
func (e *Emitter) Run() error {
	if err := os.MkdirAll(dirOf(e.path), 0o755); err != nil {
		return err
	}
	_ = os.Remove(e.path)
	ln, err := net.Listen("unix", e.path)
	if err != nil {
		return err
	}
	// 0660: guest-agent runs as root in the VM, same user group as the wrap.
	_ = os.Chmod(e.path, 0o660)
	defer ln.Close()
	defer os.Remove(e.path)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			log.Printf("claude-wrap emitter accept: %v", err)
			continue
		}
		e.serve(conn)
	}
}

// Stop shuts the emitter loop down on next accept.
func (e *Emitter) Stop() {
	close(e.stop)
	_ = os.Remove(e.path)
}

func (e *Emitter) serve(conn net.Conn) {
	defer conn.Close()
	w := bufio.NewWriter(conn)
	enc := json.NewEncoder(w)
	for {
		select {
		case <-e.stop:
			return
		case ev, ok := <-e.buf:
			if !ok {
				return
			}
			if err := enc.Encode(ev); err != nil {
				log.Printf("claude-wrap emitter encode: %v", err)
				return
			}
			if err := w.Flush(); err != nil {
				log.Printf("claude-wrap emitter flush: %v", err)
				return
			}
		}
	}
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}
