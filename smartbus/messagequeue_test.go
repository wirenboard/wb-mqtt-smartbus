package smartbus

import (
	"fmt"
	"github.com/contactless/wbgo/testutils"
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
	rec            *testutils.Recorder
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

type MessageQueueSuite struct {
	testutils.Suite
	*testutils.FakeTimerFixture
	*testutils.Recorder
	queue *MessageQueue
}

func (s *MessageQueueSuite) T() *testing.T {
	return s.Suite.T()
}

func (s *MessageQueueSuite) SetupTest() {
	s.Suite.SetupTest()

	s.Recorder = testutils.NewRecorder(s.T())
	s.FakeTimerFixture = testutils.NewFakeTimerFixture(s.T(), s.Recorder)
	s.queue = NewMessageQueue(s.NewFakeTimer, 1000*time.Millisecond, 2, 3)
	s.queue.Start()
}

func (s *MessageQueueSuite) SendMessage(opcode uint16, name string) error {
	return s.queue.Enqueue(&FakeQueueItem{s.Recorder, name, opcode})
}

func (s *MessageQueueSuite) ReceiveMessage(opcode uint16) {
	s.queue.HandleReceivedMessage(&FakeMessage{opcode})
}

func (s *MessageQueueSuite) SimulateTimeout(id int) {
	s.FireTimer(id, s.AdvanceTime(1000*time.Millisecond))
}

func (s *MessageQueueSuite) TestMessageQueue() {
	s.SendMessage(42, "forty-two")
	s.Verify("RUN: forty-two", "new fake timer: 1, 1000")
	s.ReceiveMessage(42)
	s.Verify("timer.Stop(): 1")

	// these are ignored
	s.ReceiveMessage(100)
	s.ReceiveMessage(101)

	s.SendMessage(43, "forty-three")
	s.Verify("RUN: forty-three", "new fake timer: 2, 1000")
	s.ReceiveMessage(111) // must be just skipped
	s.ReceiveMessage(43)
	s.Verify("timer.Stop(): 2")

	s.queue.Stop()
}

func (s *MessageQueueSuite) TestMessageQueueRetries() {
	s.SendMessage(42, "forty-two")
	s.Verify("RUN: forty-two", "new fake timer: 1, 1000")
	s.SimulateTimeout(1)
	s.Verify("timer.fire(): 1", "RUN: forty-two", "new fake timer: 2, 1000")
	s.ReceiveMessage(42)
	s.Verify("timer.Stop(): 2")
	s.EnsureGotWarnings()

	s.SendMessage(43, "forty-three")
	s.SendMessage(44, "forty-four")
	s.Verify("RUN: forty-three", "new fake timer: 3, 1000")
	s.SimulateTimeout(3)
	s.Verify("timer.fire(): 3", "RUN: forty-three", "new fake timer: 4, 1000")

	s.ReceiveMessage(99) // ignored
	s.ReceiveMessage(43)
	s.Verify("timer.Stop(): 4", "RUN: forty-four", "new fake timer: 5, 1000")
	s.EnsureGotWarnings()
	s.ReceiveMessage(44)
	s.Verify("timer.Stop(): 5")

	s.SendMessage(45, "forty-five")
	s.SendMessage(46, "forty-six")
	s.Verify("RUN: forty-five", "new fake timer: 6, 1000")
	// first retry
	s.SimulateTimeout(6)
	s.Verify("timer.fire(): 6", "RUN: forty-five", "new fake timer: 7, 1000")
	s.EnsureGotWarnings()
	// second and last retry
	s.SimulateTimeout(7)
	s.Verify("timer.fire(): 7", "RUN: forty-five", "new fake timer: 8, 1000")
	s.EnsureGotWarnings()
	// failed to perform the operation, go to the next message
	s.SimulateTimeout(8)
	s.Verify("timer.fire(): 8", "RUN: forty-six", "new fake timer: 9, 1000")
	s.EnsureGotErrors()
	s.ReceiveMessage(46)
	s.Verify("timer.Stop(): 9")

	s.queue.Stop()
}

func (s *MessageQueueSuite) TestMessageQueueOverflow() {
	for i := 1; i <= 5; i++ {
		name := strconv.Itoa(i)
		err := s.SendMessage(uint16(i), name)
		if i == 1 {
			s.Verify("RUN: "+name, "new fake timer: 1, 1000")
		}
		if i < 5 {
			s.Nil(err)
		} else {
			s.NotNil(err)
		}
	}

	for i := 1; i <= 3; i++ {
		name := strconv.Itoa(i)
		if i > 1 {
			s.Verify("RUN: "+name, fmt.Sprintf("new fake timer: %d, 1000", i))
		}
		s.ReceiveMessage(uint16(i))
		s.Verify(fmt.Sprintf("timer.Stop(): %d", i))
	}

	s.queue.Stop()
}

func (s *MessageQueueSuite) TestMessageQueueStoppingTimersOnStop() {
	s.SendMessage(42, "forty-two")
	s.Verify("RUN: forty-two", "new fake timer: 1, 1000")
	s.queue.Stop()
	s.Verify("timer.Stop(): 1")
	s.VerifyEmpty()
}

func (s *MessageQueueSuite) TestMessageQueueRestart() {
	s.SendMessage(42, "forty-two")
	s.Verify("RUN: forty-two", "new fake timer: 1, 1000")
	s.ReceiveMessage(42)
	s.Verify("timer.Stop(): 1")

	s.queue.Stop()
	s.queue.Start()

	s.SendMessage(43, "forty-three")
	s.Verify("RUN: forty-three", "new fake timer: 2, 1000")
	s.ReceiveMessage(43)
	s.Verify("timer.Stop(): 2")

	s.queue.Stop()
}

func TestMessageQueueSuite(t *testing.T) {
	testutils.RunSuites(t, new(MessageQueueSuite))
}
