package smartbus

import (
	"fmt"
	"github.com/contactless/wbgo"
	"sync"
	"time"
)

const (
	MESSAGE_QUEUE_INBOUND_QUEUE_SIZE = 10
)

type TimerFunc func(d time.Duration) wbgo.Timer

type QueuePred func(msg Message) bool

type QueueItem interface {
	Run()
	IsResponse(msg Message) bool
	Name() string
}

type MessageQueue struct {
	sync.Mutex
	active     bool
	items      chan QueueItem
	messages   chan Message
	quit       chan struct{}
	done       chan struct{}
	timerFunc  TimerFunc
	timeout    time.Duration
	numRetries int
}

func NewMessageQueue(timerFunc TimerFunc, timeout time.Duration, numRetries int, queueSize int) *MessageQueue {
	return &MessageQueue{
		active:     false,
		items:      make(chan QueueItem, queueSize),
		messages:   make(chan Message, MESSAGE_QUEUE_INBOUND_QUEUE_SIZE),
		quit:       nil,
		done:       make(chan struct{}),
		timerFunc:  timerFunc,
		timeout:    timeout,
		numRetries: numRetries,
	}
}

func (queue *MessageQueue) Start() {
	queue.Lock()
	defer queue.Unlock()
	if queue.active {
		return
	}
	queue.quit = make(chan struct{})
	queue.active = true
	// synchronously flush queues before returning
	queue.flush()
	go queue.run()
}

func (queue *MessageQueue) flush() {
flushLoop:
	for {
		select {
		case <-queue.items:
		case <-queue.messages:
		default:
			break flushLoop
		}
	}
}

func (queue *MessageQueue) run() {
loop:
	for {
		select {
		case <-queue.quit:
			break loop
		case <-queue.messages:
			// no pending item, items messages don't bother us
		case item := <-queue.items:
			if !queue.processItem(item) {
				// quit signalled while waiting for the response
				break loop
			}
		}
	}
	wbgo.Debug.Printf("MessageQueue: stopping the loop")
	queue.done <- struct{}{}
}

func (queue *MessageQueue) processItem(item QueueItem) bool {
	item.Run()
	if queue.timerFunc == nil {
		return true
	}
	timer := queue.timerFunc(queue.timeout)
	n := queue.numRetries
	for {
		select {
		case <-queue.quit:
			timer.Stop()
			return false
		case <-timer.GetChannel():
			if n == 0 {
				wbgo.Error.Printf(
					"command failed after %d retries: %s",
					queue.numRetries, item.Name())
				return true
			}
			n--
			wbgo.Warn.Printf("retrying %s (%d attempts left)", item.Name(), n)
			item.Run()
			timer = queue.timerFunc(queue.timeout)
		case msg := <-queue.messages:
			if item.IsResponse(msg) {
				timer.Stop()
				return true
			}
		}
	}
}

func (queue *MessageQueue) Stop() {
	queue.Lock()
	defer queue.Unlock()
	if !queue.active {
		return
	}
	queue.active = false
	close(queue.quit)
	<-queue.done
}

// Enqueue puts item into the queue. If the queue is full, an error is returned.
// This function is threadsafe.
func (queue *MessageQueue) Enqueue(item QueueItem) error {
	select {
	case queue.items <- item:
		return nil
	default:
		return fmt.Errorf("Message queue overflow, dropping request")
	}
}

// HandleReceivedMessage notifies the queue about the incoming message.
// This function is threadsafe.
func (queue *MessageQueue) HandleReceivedMessage(msg Message) {
	queue.messages <- msg
}
