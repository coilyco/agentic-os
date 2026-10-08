package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// maxCDPMessage bounds one CDP message. A screencast frame is a base64 JPEG and
// stays well under it.
const maxCDPMessage = 32 << 20

// cdpClient speaks CDP over the --remote-debugging-pipe pair: one JSON message, then
// a NUL. A pipe has no port for another local user to reach. See the browser reference.
type cdpClient struct {
	w       io.Writer
	writeMu sync.Mutex
	onEvent func(sessionID, method string, params json.RawMessage)

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan cdpReply
	// relayed are commands forwarded for a CDP peer, answered with the browser's
	// message as it came. taps see every event, in order, on the read loop.
	relayed map[int64]func(cdpMessage)
	taps    map[int]func(cdpMessage)
	nextTap int
	// done closes when the stream ends, and err says why.
	done chan struct{}
	err  error
}

type cdpReply struct {
	result json.RawMessage
	err    error
}

type cdpMessage struct {
	ID        int64           `json:"id"`
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params"`
	Result    json.RawMessage `json:"result"`
	SessionID string          `json:"sessionId"`
	Error     *cdpErrorBody   `json:"error"`
}

type cdpErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// newCDPClient reads r until it ends, delivering events to onEvent in order.
func newCDPClient(r io.Reader, w io.Writer, onEvent func(sessionID, method string, params json.RawMessage)) *cdpClient {
	c := &cdpClient{w: w, onEvent: onEvent, pending: map[int64]chan cdpReply{}, relayed: map[int64]func(cdpMessage){}, taps: map[int]func(cdpMessage){}, done: make(chan struct{})}
	go c.read(bufio.NewReaderSize(r, 1<<20))
	return c
}

func (c *cdpClient) read(r *bufio.Reader) {
	var failure error
	for {
		line, err := r.ReadBytes(0)
		if err != nil {
			failure = err
			break
		}
		if len(line) > maxCDPMessage {
			failure = errors.New("a CDP message passed its size limit")
			break
		}
		var message cdpMessage
		if err := json.Unmarshal(line[:len(line)-1], &message); err != nil {
			continue
		}
		if message.Method != "" {
			if c.onEvent != nil {
				c.onEvent(message.SessionID, message.Method, message.Params)
			}
			c.mu.Lock()
			taps := make([]func(cdpMessage), 0, len(c.taps))
			for _, tap := range c.taps {
				taps = append(taps, tap)
			}
			c.mu.Unlock()
			for _, tap := range taps {
				tap(message)
			}
			continue
		}
		c.mu.Lock()
		reply := c.pending[message.ID]
		delete(c.pending, message.ID)
		forwarded := c.relayed[message.ID]
		delete(c.relayed, message.ID)
		c.mu.Unlock()
		if forwarded != nil {
			forwarded(message)
			continue
		}
		if reply == nil {
			continue
		}
		if message.Error != nil {
			reply <- cdpReply{err: fmt.Errorf("%s (CDP %d)", message.Error.Message, message.Error.Code)}
		} else {
			reply <- cdpReply{result: message.Result}
		}
	}
	c.mu.Lock()
	c.err = failure
	for id, reply := range c.pending {
		reply <- cdpReply{err: errors.New("the browser's CDP stream ended")}
		delete(c.pending, id)
	}
	ended := c.relayed
	c.relayed = map[int64]func(cdpMessage){}
	c.mu.Unlock()
	for _, forwarded := range ended {
		forwarded(cdpMessage{Error: &cdpErrorBody{Code: -32000, Message: "the browser's CDP stream ended"}})
	}
	close(c.done)
}

// call sends one command and waits for its reply. sessionID is empty for the
// browser itself, and a flattened target's id for a page.
func (c *cdpClient) call(ctx context.Context, sessionID, method string, params any) (json.RawMessage, error) {
	encodedParams := json.RawMessage("{}")
	if params != nil {
		var err error
		if encodedParams, err = json.Marshal(params); err != nil {
			return nil, err
		}
	}
	c.mu.Lock()
	select {
	case <-c.done:
		c.mu.Unlock()
		return nil, errors.New("the browser's CDP stream ended")
	default:
	}
	c.nextID++
	id := c.nextID
	reply := make(chan cdpReply, 1)
	c.pending[id] = reply
	c.mu.Unlock()

	if err := c.write(id, sessionID, method, encodedParams); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	select {
	case answer := <-reply:
		return answer.result, answer.err
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *cdpClient) write(id int64, sessionID, method string, params json.RawMessage) error {
	request := struct {
		ID        int64           `json:"id"`
		Method    string          `json:"method"`
		Params    json.RawMessage `json:"params"`
		SessionID string          `json:"sessionId,omitempty"`
	}{id, method, params, sessionID}
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.w.Write(append(encoded, 0))
	return err
}

// relay sends a command for a CDP peer under an id of this client's own.
// done gets the whole answer on the read loop, so it must not block.
func (c *cdpClient) relay(sessionID, method string, params json.RawMessage, done func(cdpMessage)) {
	if len(params) == 0 {
		params = json.RawMessage("{}")
	}
	c.mu.Lock()
	select {
	case <-c.done:
		c.mu.Unlock()
		done(cdpMessage{Error: &cdpErrorBody{Code: -32000, Message: "the browser's CDP stream ended"}})
		return
	default:
	}
	c.nextID++
	id := c.nextID
	c.relayed[id] = done
	c.mu.Unlock()
	if err := c.write(id, sessionID, method, params); err != nil {
		c.mu.Lock()
		_, waiting := c.relayed[id]
		delete(c.relayed, id)
		c.mu.Unlock()
		if waiting {
			done(cdpMessage{Error: &cdpErrorBody{Code: -32000, Message: err.Error()}})
		}
	}
}

// tap delivers every browser event to fn until the returned stop is called.
func (c *cdpClient) tap(fn func(cdpMessage)) (stop func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextTap++
	key := c.nextTap
	c.taps[key] = fn
	return func() {
		c.mu.Lock()
		delete(c.taps, key)
		c.mu.Unlock()
	}
}

// notify sends a command whose answer nobody reads, like a frame ack.
func (c *cdpClient) notify(sessionID, method string, params any) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A canceled context returns at once, and the request is already written.
	_, _ = c.call(ctx, sessionID, method, params)
}
