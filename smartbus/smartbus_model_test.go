package smartbus

import (
	"fmt"
	"github.com/contactless/wbgo"
	"github.com/contactless/wbgo/testutils"
	"net"
	"testing"
	"time"
)

const (
	REQUEST_TIMEOUT_MS = int(REQUEST_TIMEOUT / time.Millisecond)
)

type SmartbusDriverSuiteBase struct {
	testutils.Suite
	*testutils.FakeTimerFixture
	*testutils.FakeMQTTFixture
	client    *testutils.FakeMQTTClient
	driver    *wbgo.Driver
	model     *SmartbusModel
	handler   *FakeHandler
	conn      *SmartbusConnection
	modelConn *SmartbusConnection
}

func (s *SmartbusDriverSuiteBase) T() *testing.T {
	return s.Suite.T()
}

func (s *SmartbusDriverSuiteBase) SetupTest() {
	s.Suite.SetupTest()
	s.FakeMQTTFixture = testutils.NewFakeMQTTFixture(s.T())
}

func (s *SmartbusDriverSuiteBase) Start(useTimer bool) {
	var timerFunc TimerFunc = nil
	if useTimer {
		s.FakeTimerFixture = testutils.NewFakeTimerFixture(s.T(), s.Broker.Recorder) // FIXME
		timerFunc = s.NewFakeTimer
	} else {
		s.FakeTimerFixture = nil
	}
	p, r := net.Pipe()
	s.model = NewSmartbusModel(func() (SmartbusIO, error) {
		return NewStreamIO(p, nil), nil
	}, SAMPLE_APP_SUBNET, SAMPLE_APP_DEVICE_ID, SAMPLE_APP_DEVICE_TYPE, timerFunc)
	s.client = s.Broker.MakeClient("tst")
	s.client.Start()
	s.driver = wbgo.NewDriver(s.model, s.Broker.MakeClient("driver"))
	s.driver.SetAutoPoll(false)

	s.handler = NewFakeHandler(s.T())
	s.conn = NewSmartbusConnection(NewStreamIO(r, nil))
}

func (s *SmartbusDriverSuiteBase) TearDownTest() {
	s.driver.Stop()
	s.conn.Close()
	s.Verify(
		"stop: driver",
	)
	s.Suite.TearDownTest()
}

func (s *SmartbusDriverSuiteBase) VerifyVirtualRelays() {
	expected := make([]interface{}, 0, 100)
	expected = append(
		expected,
		"driver -> /devices/sbusvrelay/meta/name: [Smartbus Virtual Relays] (QoS 1, retained)")
	for i := 1; i <= NUM_VIRTUAL_RELAYS; i++ {
		path := fmt.Sprintf("/devices/sbusvrelay/controls/VirtualRelay%d", i)
		expected = append(
			expected,
			fmt.Sprintf("driver -> %s/meta/type: [switch] (QoS 1, retained)", path),
			fmt.Sprintf("driver -> %s/meta/readonly: [1] (QoS 1, retained)", path),
			fmt.Sprintf("driver -> %s/meta/order: [%d] (QoS 1, retained)", path, i),
			fmt.Sprintf("driver -> %s: [0] (QoS 1, retained)", path),
		)
	}
	s.Verify(expected...)
}

type DDPSuite struct {
	SmartbusDriverSuiteBase
	ddpEp       *SmartbusEndpoint
	ddpToAppDev *SmartbusDevice
}

func (s *DDPSuite) Start(useTimer bool) {
	s.SmartbusDriverSuiteBase.Start(useTimer)
	s.ddpEp = s.conn.MakeSmartbusEndpoint(
		SAMPLE_SUBNET, SAMPLE_DDP_DEVICE_ID, SAMPLE_DDP_DEVICE_TYPE)
	s.ddpEp.Observe(s.handler)
	s.ddpToAppDev = s.ddpEp.GetSmartbusDevice(SAMPLE_APP_SUBNET, SAMPLE_APP_DEVICE_ID)

	s.driver.Start()
	s.VerifyVirtualRelays()
	s.detectIt()
	s.verifyQueryingButtons(useTimer)
}

func (s *DDPSuite) detectIt() {
	s.handler.Verify("03/fe (type fffe) -> ff/ff: <ReadMACAddress>")
	s.ddpToAppDev.ReadMACAddressResponse(
		[8]byte{
			0x53, 0x03, 0x00, 0x00,
			0x00, 0x00, 0x30, 0xc3,
		},
		[]uint8{
			0x20, 0x42, 0x42,
		})
	s.Verify(
		"driver -> /devices/ddp0114/meta/name: [DDP 01:14] (QoS 1, retained)")
}

func (s *DDPSuite) verifyQueryingButtons(useTimer bool) {
	for i := 1; i <= PANEL_BUTTON_COUNT; i++ {
		s.handler.Verify(fmt.Sprintf(
			"03/fe (type fffe) -> 01/14: <QueryPanelButtonAssignment %d/1>", i))
		if useTimer {
			s.Verify(fmt.Sprintf("new fake timer: %d, %d", i, REQUEST_TIMEOUT_MS))
		}
		assignment := -1
		if i <= 10 {
			s.ddpToAppDev.QueryPanelButtonAssignmentResponse(
				uint8(i), 1, BUTTON_COMMAND_INVALID, 0, 0, 0, 0, 0)
		} else {
			assignment = i - 10
			s.ddpToAppDev.QueryPanelButtonAssignmentResponse(
				uint8(i), 1, BUTTON_COMMAND_SINGLE_CHANNEL_LIGHTING_CONTROL,
				SAMPLE_APP_SUBNET, SAMPLE_APP_DEVICE_ID,
				uint8(assignment), 100, 0)
		}
		path := fmt.Sprintf("/devices/ddp0114/controls/Page%dButton%d",
			(i-1)/4+1, (i-1)%4+1)
		items := []interface{}{
			fmt.Sprintf("driver -> %s/meta/type: [text] (QoS 1, retained)", path),
			fmt.Sprintf("driver -> %s/meta/order: [%d] (QoS 1, retained)", path, i),
			fmt.Sprintf("driver -> %s: [%d] (QoS 1, retained)", path, assignment),
			fmt.Sprintf("Subscribe -- driver: %s/on", path),
		}
		if useTimer {
			items := append([]interface{}{
				fmt.Sprintf("timer.Stop(): %d", i),
			}, items...)
			s.VerifyUnordered(items...)
		} else {
			s.Verify(items...)
		}
	}
	if useTimer {
		s.ResetTimerIndex()
	}
}

func (s *DDPSuite) TestSmartbusDriverDDPHandling() {
	s.Start(false)

	// second QueryModules shouldn't cause anything
	s.ddpToAppDev.QueryModules()
	s.handler.Verify()
	s.Verify()

	s.client.Publish(
		wbgo.MQTTMessage{"/devices/ddp0114/controls/Page1Button2/on", "10", 1, false})

	s.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SetPanelButtonModes " +
		"1/1:Invalid,1/2:SingleOnOff,1/3:Invalid,1/4:Invalid," +
		"2/1:Invalid,2/2:Invalid,2/3:Invalid,2/4:Invalid," +
		"3/1:Invalid,3/2:Invalid,3/3:SingleOnOff,3/4:SingleOnOff," +
		"4/1:SingleOnOff,4/2:SingleOnOff,4/3:SingleOnOff,4/4:SingleOnOff>")
	s.ddpToAppDev.SetPanelButtonModesResponse(true)
	s.Verify("tst -> /devices/ddp0114/controls/Page1Button2/on: [10] (QoS 1)")
	s.handler.Verify("03/fe (type fffe) -> 01/14: <AssignPanelButton 2/1/59/03/fe/10/100/0/0>")
	s.ddpToAppDev.AssignPanelButtonResponse(2, 1)
	s.Verify("driver -> /devices/ddp0114/controls/Page1Button2: [10] (QoS 1, retained)")

	s.ddpToAppDev.SingleChannelControl(10, LIGHT_LEVEL_ON, 0)
	s.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SingleChannelControlResponse 10/true/100/" +
		"---------x----->")
	s.Verify(
		"driver -> /devices/sbusvrelay/controls/VirtualRelay10: [1] (QoS 1, retained)")

	s.ddpToAppDev.SingleChannelControl(12, LIGHT_LEVEL_ON, 0)
	s.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SingleChannelControlResponse 12/true/100/" +
		"---------x-x--->")
	s.Verify(
		"driver -> /devices/sbusvrelay/controls/VirtualRelay12: [1] (QoS 1, retained)")

	s.ddpToAppDev.SingleChannelControl(12, LIGHT_LEVEL_OFF, 0)
	s.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SingleChannelControlResponse 12/true/0/" +
		"---------x----->")
	s.Verify(
		"driver -> /devices/sbusvrelay/controls/VirtualRelay12: [0] (QoS 1, retained)")

	s.ddpToAppDev.SingleChannelControl(10, LIGHT_LEVEL_OFF, 0)
	s.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SingleChannelControlResponse 10/true/0/" +
		"--------------->")
	s.Verify(
		"driver -> /devices/sbusvrelay/controls/VirtualRelay10: [0] (QoS 1, retained)")
}

func (s *DDPSuite) TestSmartbusDriverDDPCommandQueue() {
	s.Start(true)

	s.client.Publish(
		wbgo.MQTTMessage{"/devices/ddp0114/controls/Page1Button2/on", "10", 1, false})
	s.Verify("tst -> /devices/ddp0114/controls/Page1Button2/on: [10] (QoS 1)")
	s.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SetPanelButtonModes " +
		"1/1:Invalid,1/2:SingleOnOff,1/3:Invalid,1/4:Invalid," +
		"2/1:Invalid,2/2:Invalid,2/3:Invalid,2/4:Invalid," +
		"3/1:Invalid,3/2:Invalid,3/3:SingleOnOff,3/4:SingleOnOff," +
		"4/1:SingleOnOff,4/2:SingleOnOff,4/3:SingleOnOff,4/4:SingleOnOff>")
	s.Verify(fmt.Sprintf("new fake timer: 1, %d", REQUEST_TIMEOUT_MS))

	s.FireTimer(1, s.AdvanceTime(1000))
	s.Verify("timer.fire(): 1")
	s.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SetPanelButtonModes " +
		"1/1:Invalid,1/2:SingleOnOff,1/3:Invalid,1/4:Invalid," +
		"2/1:Invalid,2/2:Invalid,2/3:Invalid,2/4:Invalid," +
		"3/1:Invalid,3/2:Invalid,3/3:SingleOnOff,3/4:SingleOnOff," +
		"4/1:SingleOnOff,4/2:SingleOnOff,4/3:SingleOnOff,4/4:SingleOnOff>")
	s.Verify(fmt.Sprintf("new fake timer: 2, %d", REQUEST_TIMEOUT_MS))
	s.EnsureGotWarnings()

	s.ddpToAppDev.SetPanelButtonModesResponse(true)
	s.Verify(
		"timer.Stop(): 2",
	)
	s.handler.Verify("03/fe (type fffe) -> 01/14: <AssignPanelButton 2/1/59/03/fe/10/100/0/0>")
	s.Verify(fmt.Sprintf("new fake timer: 3, %d", REQUEST_TIMEOUT_MS))
	s.ddpToAppDev.AssignPanelButtonResponse(2, 1)
	s.VerifyUnordered(
		"timer.Stop(): 3",
		"driver -> /devices/ddp0114/controls/Page1Button2: [10] (QoS 1, retained)")
}

type ZoneBeastSuite struct {
	SmartbusDriverSuiteBase
	relayEp       *SmartbusEndpoint
	relayToAllDev *SmartbusDevice
	relayToAppDev *SmartbusDevice
}

func (s *ZoneBeastSuite) Start(useTimer bool) {
	s.SmartbusDriverSuiteBase.Start(useTimer)

	s.relayEp = s.conn.MakeSmartbusEndpoint(
		SAMPLE_SUBNET, SAMPLE_RELAY_DEVICE_ID, SAMPLE_RELAY_DEVICE_TYPE)
	s.relayEp.Observe(s.handler)
	s.relayToAllDev = s.relayEp.GetBroadcastDevice()
	s.relayToAppDev = s.relayEp.GetSmartbusDevice(
		SAMPLE_APP_SUBNET, SAMPLE_APP_DEVICE_ID)

	s.driver.Start()
	s.VerifyVirtualRelays()
	s.detectIt()
	s.firstBroadcast()
	return
}

func (s *ZoneBeastSuite) detectIt() {
	s.handler.Verify("03/fe (type fffe) -> ff/ff: <ReadMACAddress>")
	s.relayToAppDev.ReadMACAddressResponse(
		[8]byte{
			0x53, 0x03, 0x00, 0x00,
			0x00, 0x00, 0x42, 0x42,
		},
		[]uint8{})
	s.Verify(
		"driver -> /devices/zonebeast011c/meta/name: [Zone Beast 01:1c] (QoS 1, retained)",
	)
}

func (s *ZoneBeastSuite) firstBroadcast() {
	s.relayToAllDev.ZoneBeastBroadcast([]byte{0}, parseChannelStatus("---x"))
	s.Verify(
		"driver -> /devices/zonebeast011c/controls/Channel 1/meta/type: [switch] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Channel 1/meta/order: [1] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Channel 1: [0] (QoS 1, retained)",
		"Subscribe -- driver: /devices/zonebeast011c/controls/Channel 1/on",

		"driver -> /devices/zonebeast011c/controls/Channel 2/meta/type: [switch] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Channel 2/meta/order: [2] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Channel 2: [0] (QoS 1, retained)",
		"Subscribe -- driver: /devices/zonebeast011c/controls/Channel 2/on",

		"driver -> /devices/zonebeast011c/controls/Channel 3/meta/type: [switch] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Channel 3/meta/order: [3] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Channel 3: [0] (QoS 1, retained)",
		"Subscribe -- driver: /devices/zonebeast011c/controls/Channel 3/on",

		"driver -> /devices/zonebeast011c/controls/Channel 4/meta/type: [switch] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Channel 4/meta/order: [4] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Channel 4: [1] (QoS 1, retained)",
		"Subscribe -- driver: /devices/zonebeast011c/controls/Channel 4/on",
	)
}

func (s *ZoneBeastSuite) TestSmartbusDriverZoneBeastHandling() {
	s.Start(false)

	s.relayToAllDev.ZoneBeastBroadcast([]byte{0}, parseChannelStatus("x---"))
	s.Verify(
		"driver -> /devices/zonebeast011c/controls/Channel 1: [1] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Channel 4: [0] (QoS 1, retained)",
	)

	s.client.Publish(
		wbgo.MQTTMessage{"/devices/zonebeast011c/controls/Channel 2/on", "1", 1, false})
	// note that SingleChannelControlResponse carries pre-command channel status
	s.handler.Verify(
		"03/fe (type fffe) -> 01/1c: <SingleChannelControlCommand 2/100/0>")
	s.relayToAllDev.SingleChannelControlResponse(2, true, LIGHT_LEVEL_ON, parseChannelStatus("x---"))
	s.Verify(
		"tst -> /devices/zonebeast011c/controls/Channel 2/on: [1] (QoS 1)",
		"driver -> /devices/zonebeast011c/controls/Channel 2: [1] (QoS 1, retained)",
	)

	s.client.Publish(
		wbgo.MQTTMessage{"/devices/zonebeast011c/controls/Channel 1/on", "0", 1, false})
	s.handler.Verify(
		"03/fe (type fffe) -> 01/1c: <SingleChannelControlCommand 1/0/0>")
	s.relayToAllDev.SingleChannelControlResponse(1, true, LIGHT_LEVEL_OFF, parseChannelStatus("xx--"))
	s.relayToAllDev.ZoneBeastBroadcast([]byte{0}, parseChannelStatus("x---")) // outdated response -- must be ignored
	s.Verify(
		"tst -> /devices/zonebeast011c/controls/Channel 1/on: [0] (QoS 1)",
		"driver -> /devices/zonebeast011c/controls/Channel 1: [0] (QoS 1, retained)",
	)

	s.driver.Poll()
	s.handler.Verify("03/fe (type fffe) -> 01/1c: <ReadTemperatureValues Celsius>")
	s.relayToAllDev.ReadTemperatureValuesResponse(true, []int8{22})
	s.Verify(
		"driver -> /devices/zonebeast011c/controls/Temp 1/meta/type: [temperature] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Temp 1/meta/readonly: [1] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Temp 1/meta/order: [5] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Temp 1: [22] (QoS 1, retained)",
	)

	s.driver.Poll()
	s.handler.Verify("03/fe (type fffe) -> 01/1c: <ReadTemperatureValues Celsius>")
	s.relayToAllDev.ReadTemperatureValuesResponse(true, []int8{-2})
	s.Verify(
		"driver -> /devices/zonebeast011c/controls/Temp 1: [-2] (QoS 1, retained)",
	)
}

func (s *ZoneBeastSuite) TestSmartbusDriverZoneBeastCommandQueue() {
	s.Start(true)

	s.client.Publish(
		wbgo.MQTTMessage{"/devices/zonebeast011c/controls/Channel 2/on", "1", 1, false})
	s.Verify(
		"tst -> /devices/zonebeast011c/controls/Channel 2/on: [1] (QoS 1)",
	)
	s.handler.Verify(
		"03/fe (type fffe) -> 01/1c: <SingleChannelControlCommand 2/100/0>")
	s.Verify(
		fmt.Sprintf("new fake timer: 1, %d", REQUEST_TIMEOUT_MS),
	)

	s.FireTimer(1, s.AdvanceTime(1000)) // oops, timeout!
	s.Verify("timer.fire(): 1")
	s.handler.Verify(
		"03/fe (type fffe) -> 01/1c: <SingleChannelControlCommand 2/100/0>")
	s.Verify(
		fmt.Sprintf("new fake timer: 2, %d", REQUEST_TIMEOUT_MS),
	)
	s.EnsureGotWarnings()

	// note that SingleChannelControlResponse carries pre-command channel status
	s.relayToAllDev.SingleChannelControlResponse(2, true, LIGHT_LEVEL_ON, parseChannelStatus("x---"))
	s.VerifyUnordered(
		"timer.Stop(): 2",
		"driver -> /devices/zonebeast011c/controls/Channel 2: [1] (QoS 1, retained)",
	)
}

func TestSmartbusDriverSuite(t *testing.T) {
	testutils.RunSuites(t, new(DDPSuite), new(ZoneBeastSuite))
}

// TBD: outdated ZoneBeastBroadcast messages still arrive sometimes, need to fix this
