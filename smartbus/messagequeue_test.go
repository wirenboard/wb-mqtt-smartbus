package smartbus

import (
	"fmt"
	"github.com/contactless/wbgo"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
	"time"
)

type FakeMessage struct {
	opcode uint16
}

func (msg *FakeMessage) Opcode() uint16 {
	return msg.opcode
}

type FakeQueueItem struct {
	rec            *wbgo.Recorder
	name           string
	expectedOpcode uint16
}

func (item *FakeQueueItem) Run() {
	item.rec.Rec("RUN: %s", item.name)
}

func (item *FakeQueueItem) IsResponse(msg Message) bool {
	return item.expectedOpcode == msg.Opcode()
}

func (item *FakeQueueItem) Name() string {
	return item.name
}

type messageQueueFixture struct {
	*wbgo.FakeTimerFixture
	*wbgo.Recorder
	queue *MessageQueue
}

func newMessageFixture(t *testing.T) *messageQueueFixture {
	wbgo.SetupTestLogging(t)

	rec := wbgo.NewRecorder(t)
	timerFixture := wbgo.NewFakeTimerFixture(t, rec)
	queue := NewMessageQueue(timerFixture.NewFakeTimer, 1000*time.Millisecond, 2, 3)
	queue.Start()
	return &messageQueueFixture{timerFixture, rec, queue}
}

func (fixture *messageQueueFixture) SendMessage(opcode uint16, name string) error {
	return fixture.queue.Enqueue(&FakeQueueItem{fixture.Recorder, name, opcode})
}

func (fixture *messageQueueFixture) ReceiveMessage(opcode uint16) {
	fixture.queue.HandleReceivedMessage(&FakeMessage{opcode})
}

func (fixture *messageQueueFixture) SimulateTimeout(id int) {
	fixture.FireTimer(id, fixture.AdvanceTime(1000*time.Millisecond))
}

func TestMessageQueue(t *testing.T) {
	fixture := newMessageFixture(t)
	fixture.SendMessage(42, "forty-two")
	fixture.Verify("RUN: forty-two", "new fake timer: 1, 1000")
	fixture.ReceiveMessage(42)
	fixture.Verify("timer.Stop(): 1")

	// these are ignored
	fixture.ReceiveMessage(100)
	fixture.ReceiveMessage(101)

	fixture.SendMessage(43, "forty-three")
	fixture.Verify("RUN: forty-three", "new fake timer: 2, 1000")
	fixture.ReceiveMessage(111) // must be just skipped
	fixture.ReceiveMessage(43)
	fixture.Verify("timer.Stop(): 2")

	fixture.queue.Stop()
	wbgo.EnsureNoErrorsOrWarnings(t)
}

func TestMessageQueueRetries(t *testing.T) {
	fixture := newMessageFixture(t)

	fixture.SendMessage(42, "forty-two")
	fixture.Verify("RUN: forty-two", "new fake timer: 1, 1000")
	fixture.SimulateTimeout(1)
	fixture.Verify("timer.fire(): 1", "RUN: forty-two", "new fake timer: 2, 1000")
	fixture.ReceiveMessage(42)
	fixture.Verify("timer.Stop(): 2")
	wbgo.EnsureGotWarnings(t)

	fixture.SendMessage(43, "forty-three")
	fixture.SendMessage(44, "forty-four")
	fixture.Verify("RUN: forty-three", "new fake timer: 3, 1000")
	fixture.SimulateTimeout(3)
	fixture.Verify("timer.fire(): 3", "RUN: forty-three", "new fake timer: 4, 1000")

	fixture.ReceiveMessage(99) // ignored
	fixture.ReceiveMessage(43)
	fixture.Verify("timer.Stop(): 4", "RUN: forty-four", "new fake timer: 5, 1000")
	wbgo.EnsureGotWarnings(t)
	fixture.ReceiveMessage(44)
	fixture.Verify("timer.Stop(): 5")

	fixture.SendMessage(45, "forty-five")
	fixture.SendMessage(46, "forty-six")
	fixture.Verify("RUN: forty-five", "new fake timer: 6, 1000")
	// first retry
	fixture.SimulateTimeout(6)
	fixture.Verify("timer.fire(): 6", "RUN: forty-five", "new fake timer: 7, 1000")
	wbgo.EnsureGotWarnings(t)
	// second and last retry
	fixture.SimulateTimeout(7)
	fixture.Verify("timer.fire(): 7", "RUN: forty-five", "new fake timer: 8, 1000")
	wbgo.EnsureGotWarnings(t)
	// failed to perform the operation, go to the next message
	fixture.SimulateTimeout(8)
	fixture.Verify("timer.fire(): 8", "RUN: forty-six", "new fake timer: 9, 1000")
	wbgo.EnsureGotErrors(t)
	fixture.ReceiveMessage(46)
	fixture.Verify("timer.Stop(): 9")

	fixture.queue.Stop()
	wbgo.EnsureNoErrorsOrWarnings(t)
}

func TestMessageQueueOverflow(t *testing.T) {
	fixture := newMessageFixture(t)

	for i := 1; i <= 5; i++ {
		name := strconv.Itoa(i)
		err := fixture.SendMessage(uint16(i), name)
		if i == 1 {
			fixture.Verify("RUN: "+name, "new fake timer: 1, 1000")
		}
		if i < 5 {
			assert.Nil(t, err)
		} else {
			assert.NotNil(t, err)
		}
	}

	for i := 1; i <= 3; i++ {
		name := strconv.Itoa(i)
		if i > 1 {
			fixture.Verify("RUN: "+name, fmt.Sprintf("new fake timer: %d, 1000", i))
		}
		fixture.ReceiveMessage(uint16(i))
		fixture.Verify(fmt.Sprintf("timer.Stop(): %d", i))
	}

	fixture.queue.Stop()
	wbgo.EnsureNoErrorsOrWarnings(t)
}

func TestMessageQueueStoppingTimersOnStop(t *testing.T) {
	fixture := newMessageFixture(t)
	fixture.SendMessage(42, "forty-two")
	fixture.Verify("RUN: forty-two", "new fake timer: 1, 1000")
	fixture.queue.Stop()
	fixture.Verify("timer.Stop(): 1")
	fixture.VerifyEmpty()
	wbgo.EnsureNoErrorsOrWarnings(t)
}

func TestMessageQueueRestart(t *testing.T) {
	fixture := newMessageFixture(t)
	fixture.SendMessage(42, "forty-two")
	fixture.Verify("RUN: forty-two", "new fake timer: 1, 1000")
	fixture.ReceiveMessage(42)
	fixture.Verify("timer.Stop(): 1")

	fixture.queue.Stop()
	fixture.queue.Start()

	fixture.SendMessage(43, "forty-three")
	fixture.Verify("RUN: forty-three", "new fake timer: 2, 1000")
	fixture.ReceiveMessage(43)
	fixture.Verify("timer.Stop(): 2")

	fixture.queue.Stop()
	wbgo.EnsureNoErrorsOrWarnings(t)
}
