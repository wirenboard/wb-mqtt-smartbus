package smartbus

import (
	"fmt"
	"github.com/contactless/wbgo"
	"net"
	"testing"
)

type smartbusDriverFixture struct {
	*wbgo.FakeTimerFixture
	t       *testing.T
	client  *wbgo.FakeMQTTClient
	broker  *wbgo.FakeMQTTBroker
	driver  *wbgo.Driver
	model   *SmartbusModel
	handler *FakeHandler
	conn    *SmartbusConnection
}

func newSmartbusDriverFixture(t *testing.T, useTimer bool) *smartbusDriverFixture {
	wbgo.SetupTestLogging(t)

	p, r := net.Pipe()

	broker := wbgo.NewFakeMQTTBroker(t)
	var fakeTimerFixture *wbgo.FakeTimerFixture
	timerFunc := TimerFunc(nil)
	if useTimer {
		fakeTimerFixture = wbgo.NewFakeTimerFixture(t, &broker.Recorder)
		timerFunc = fakeTimerFixture.NewFakeTimer
	}
	model := NewSmartbusModel(func() (SmartbusIO, error) {
		return NewStreamIO(p, nil), nil
	}, SAMPLE_APP_SUBNET, SAMPLE_APP_DEVICE_ID, SAMPLE_APP_DEVICE_TYPE,
		timerFunc)
	client := broker.MakeClient("tst")
	client.Start()
	driver := wbgo.NewDriver(model, broker.MakeClient("driver"))
	driver.SetAutoPoll(false)

	handler := NewFakeHandler(t)
	conn := NewSmartbusConnection(NewStreamIO(r, nil))
	return &smartbusDriverFixture{fakeTimerFixture, t, client, broker, driver, model, handler, conn}
}

func (fixture *smartbusDriverFixture) tearDown() {
	fixture.driver.Stop()
	fixture.conn.Close()
	fixture.Verify(
		"stop: driver",
	)
	wbgo.EnsureNoErrorsOrWarnings(fixture.t)
}

func (fixture *smartbusDriverFixture) Verify(expected ...string) {
	fixture.broker.Verify(expected...)
}

func (fixture *smartbusDriverFixture) VerifyUnordered(expected ...string) {
	fixture.broker.VerifyUnordered(expected...)
}

func (fixture *smartbusDriverFixture) VerifyVirtualRelays() {
	expected := make([]string, 0, 100)
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
	fixture.Verify(expected...)
}

type ddpFixture struct {
	*smartbusDriverFixture
	ddpEp       *SmartbusEndpoint
	ddpToAppDev *SmartbusDevice
}

func newDDPFixture(t *testing.T, useTimer bool) (fixture *ddpFixture) {
	fixture = &ddpFixture{smartbusDriverFixture: newSmartbusDriverFixture(t, useTimer)}
	fixture.ddpEp = fixture.conn.MakeSmartbusEndpoint(
		SAMPLE_SUBNET, SAMPLE_DDP_DEVICE_ID, SAMPLE_DDP_DEVICE_TYPE)
	fixture.ddpEp.Observe(fixture.handler)
	fixture.ddpToAppDev = fixture.ddpEp.GetSmartbusDevice(SAMPLE_APP_SUBNET, SAMPLE_APP_DEVICE_ID)

	fixture.driver.Start()
	fixture.VerifyVirtualRelays()
	fixture.detectIt()
	fixture.verifyQueryingButtons(useTimer)
	// TBD: reset timer index
	return
}

func (fixture *ddpFixture) detectIt() {
	fixture.handler.Verify("03/fe (type fffe) -> ff/ff: <ReadMACAddress>")
	fixture.ddpToAppDev.ReadMACAddressResponse(
		[8]byte{
			0x53, 0x03, 0x00, 0x00,
			0x00, 0x00, 0x30, 0xc3,
		},
		[]uint8{
			0x20, 0x42, 0x42,
		})
	fixture.Verify(
		"driver -> /devices/ddp0114/meta/name: [DDP 01:14] (QoS 1, retained)")
}

func (fixture *ddpFixture) verifyQueryingButtons(useTimer bool) {
	for i := 1; i <= PANEL_BUTTON_COUNT; i++ {
		fixture.handler.Verify(fmt.Sprintf(
			"03/fe (type fffe) -> 01/14: <QueryPanelButtonAssignment %d/1>", i))
		if useTimer {
			fixture.Verify(fmt.Sprintf("new fake timer: %d, 1000", i))
		}
		assignment := -1
		if i <= 10 {
			fixture.ddpToAppDev.QueryPanelButtonAssignmentResponse(
				uint8(i), 1, BUTTON_COMMAND_INVALID, 0, 0, 0, 0, 0)
		} else {
			assignment = i - 10
			fixture.ddpToAppDev.QueryPanelButtonAssignmentResponse(
				uint8(i), 1, BUTTON_COMMAND_SINGLE_CHANNEL_LIGHTING_CONTROL,
				SAMPLE_APP_SUBNET, SAMPLE_APP_DEVICE_ID,
				uint8(assignment), 100, 0)
		}
		path := fmt.Sprintf("/devices/ddp0114/controls/Page%dButton%d",
			(i-1)/4+1, (i-1)%4+1)
		items := []string{
			fmt.Sprintf("driver -> %s/meta/type: [text] (QoS 1, retained)", path),
			fmt.Sprintf("driver -> %s/meta/order: [%d] (QoS 1, retained)", path, i),
			fmt.Sprintf("driver -> %s: [%d] (QoS 1, retained)", path, assignment),
			fmt.Sprintf("Subscribe -- driver: %s/on", path),
		}
		if useTimer {
			items := append([]string{
				fmt.Sprintf("timer.Stop(): %d", i),
			}, items...)
			fixture.VerifyUnordered(items...)
		} else {
			fixture.Verify(items...)
		}
	}
	if fixture.FakeTimerFixture != nil {
		fixture.ResetTimerIndex()
	}
}

func TestSmartbusDriverDDPHandling(t *testing.T) {
	fixture := newDDPFixture(t, false)
	defer fixture.tearDown()

	// second QueryModules shouldn't cause anything
	fixture.ddpToAppDev.QueryModules()
	fixture.handler.Verify()
	fixture.Verify()

	fixture.client.Publish(
		wbgo.MQTTMessage{"/devices/ddp0114/controls/Page1Button2/on", "10", 1, false})

	fixture.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SetPanelButtonModes " +
		"1/1:Invalid,1/2:SingleOnOff,1/3:Invalid,1/4:Invalid," +
		"2/1:Invalid,2/2:Invalid,2/3:Invalid,2/4:Invalid," +
		"3/1:Invalid,3/2:Invalid,3/3:SingleOnOff,3/4:SingleOnOff," +
		"4/1:SingleOnOff,4/2:SingleOnOff,4/3:SingleOnOff,4/4:SingleOnOff>")
	fixture.ddpToAppDev.SetPanelButtonModesResponse(true)
	fixture.Verify("tst -> /devices/ddp0114/controls/Page1Button2/on: [10] (QoS 1)")
	fixture.handler.Verify("03/fe (type fffe) -> 01/14: <AssignPanelButton 2/1/59/03/fe/10/100/0/0>")
	fixture.ddpToAppDev.AssignPanelButtonResponse(2, 1)
	fixture.Verify("driver -> /devices/ddp0114/controls/Page1Button2: [10] (QoS 1, retained)")

	fixture.ddpToAppDev.SingleChannelControl(10, LIGHT_LEVEL_ON, 0)
	fixture.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SingleChannelControlResponse 10/true/100/" +
		"---------x----->")
	fixture.Verify(
		"driver -> /devices/sbusvrelay/controls/VirtualRelay10: [1] (QoS 1, retained)")

	fixture.ddpToAppDev.SingleChannelControl(12, LIGHT_LEVEL_ON, 0)
	fixture.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SingleChannelControlResponse 12/true/100/" +
		"---------x-x--->")
	fixture.Verify(
		"driver -> /devices/sbusvrelay/controls/VirtualRelay12: [1] (QoS 1, retained)")

	fixture.ddpToAppDev.SingleChannelControl(12, LIGHT_LEVEL_OFF, 0)
	fixture.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SingleChannelControlResponse 12/true/0/" +
		"---------x----->")
	fixture.Verify(
		"driver -> /devices/sbusvrelay/controls/VirtualRelay12: [0] (QoS 1, retained)")

	fixture.ddpToAppDev.SingleChannelControl(10, LIGHT_LEVEL_OFF, 0)
	fixture.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SingleChannelControlResponse 10/true/0/" +
		"--------------->")
	fixture.broker.Verify(
		"driver -> /devices/sbusvrelay/controls/VirtualRelay10: [0] (QoS 1, retained)")
}

func TestSmartbusDriverDDPCommandQueue(t *testing.T) {
	fixture := newDDPFixture(t, true)
	defer fixture.tearDown()

	fixture.client.Publish(
		wbgo.MQTTMessage{"/devices/ddp0114/controls/Page1Button2/on", "10", 1, false})
	fixture.Verify("tst -> /devices/ddp0114/controls/Page1Button2/on: [10] (QoS 1)")
	fixture.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SetPanelButtonModes " +
		"1/1:Invalid,1/2:SingleOnOff,1/3:Invalid,1/4:Invalid," +
		"2/1:Invalid,2/2:Invalid,2/3:Invalid,2/4:Invalid," +
		"3/1:Invalid,3/2:Invalid,3/3:SingleOnOff,3/4:SingleOnOff," +
		"4/1:SingleOnOff,4/2:SingleOnOff,4/3:SingleOnOff,4/4:SingleOnOff>")
	fixture.Verify("new fake timer: 1, 1000")

	fixture.FireTimer(1, fixture.AdvanceTime(1000))
	fixture.Verify("timer.fire(): 1")
	fixture.handler.Verify("03/fe (type fffe) -> 01/14: " +
		"<SetPanelButtonModes " +
		"1/1:Invalid,1/2:SingleOnOff,1/3:Invalid,1/4:Invalid," +
		"2/1:Invalid,2/2:Invalid,2/3:Invalid,2/4:Invalid," +
		"3/1:Invalid,3/2:Invalid,3/3:SingleOnOff,3/4:SingleOnOff," +
		"4/1:SingleOnOff,4/2:SingleOnOff,4/3:SingleOnOff,4/4:SingleOnOff>")
	fixture.Verify("new fake timer: 2, 1000")
	wbgo.EnsureGotWarnings(t)

	fixture.ddpToAppDev.SetPanelButtonModesResponse(true)
	fixture.Verify(
		"timer.Stop(): 2",
	)
	fixture.handler.Verify("03/fe (type fffe) -> 01/14: <AssignPanelButton 2/1/59/03/fe/10/100/0/0>")
	fixture.Verify("new fake timer: 3, 1000")
	fixture.ddpToAppDev.AssignPanelButtonResponse(2, 1)
	fixture.VerifyUnordered(
		"timer.Stop(): 3",
		"driver -> /devices/ddp0114/controls/Page1Button2: [10] (QoS 1, retained)")
}

type zoneBeastFixture struct {
	*smartbusDriverFixture
	relayEp       *SmartbusEndpoint
	relayToAllDev *SmartbusDevice
	relayToAppDev *SmartbusDevice
}

func newZoneBeastFixture(t *testing.T, useTimer bool) (fixture *zoneBeastFixture) {
	fixture = &zoneBeastFixture{smartbusDriverFixture: newSmartbusDriverFixture(t, useTimer)}

	fixture.relayEp = fixture.conn.MakeSmartbusEndpoint(
		SAMPLE_SUBNET, SAMPLE_RELAY_DEVICE_ID, SAMPLE_RELAY_DEVICE_TYPE)
	fixture.relayEp.Observe(fixture.handler)
	fixture.relayToAllDev = fixture.relayEp.GetBroadcastDevice()
	fixture.relayToAppDev = fixture.relayEp.GetSmartbusDevice(
		SAMPLE_APP_SUBNET, SAMPLE_APP_DEVICE_ID)

	fixture.driver.Start()
	fixture.VerifyVirtualRelays()
	fixture.detectIt()
	fixture.firstBroadcast()
	return
}

func (fixture *zoneBeastFixture) detectIt() {
	fixture.handler.Verify("03/fe (type fffe) -> ff/ff: <ReadMACAddress>")
	fixture.relayToAppDev.ReadMACAddressResponse(
		[8]byte{
			0x53, 0x03, 0x00, 0x00,
			0x00, 0x00, 0x42, 0x42,
		},
		[]uint8{})
	fixture.Verify(
		"driver -> /devices/zonebeast011c/meta/name: [Zone Beast 01:1c] (QoS 1, retained)",
	)
}

func (fixture *zoneBeastFixture) firstBroadcast() {
	fixture.relayToAllDev.ZoneBeastBroadcast([]byte{0}, parseChannelStatus("---x"))
	fixture.Verify(
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

func TestSmartbusDriverZoneBeastHandling(t *testing.T) {
	fixture := newZoneBeastFixture(t, false)
	defer fixture.tearDown()

	fixture.relayToAllDev.ZoneBeastBroadcast([]byte{0}, parseChannelStatus("x---"))
	fixture.Verify(
		"driver -> /devices/zonebeast011c/controls/Channel 1: [1] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Channel 4: [0] (QoS 1, retained)",
	)

	fixture.client.Publish(
		wbgo.MQTTMessage{"/devices/zonebeast011c/controls/Channel 2/on", "1", 1, false})
	// note that SingleChannelControlResponse carries pre-command channel status
	fixture.handler.Verify(
		"03/fe (type fffe) -> 01/1c: <SingleChannelControlCommand 2/100/0>")
	fixture.relayToAllDev.SingleChannelControlResponse(2, true, LIGHT_LEVEL_ON, parseChannelStatus("x---"))
	fixture.Verify(
		"tst -> /devices/zonebeast011c/controls/Channel 2/on: [1] (QoS 1)",
		"driver -> /devices/zonebeast011c/controls/Channel 2: [1] (QoS 1, retained)",
	)

	fixture.client.Publish(
		wbgo.MQTTMessage{"/devices/zonebeast011c/controls/Channel 1/on", "0", 1, false})
	fixture.handler.Verify(
		"03/fe (type fffe) -> 01/1c: <SingleChannelControlCommand 1/0/0>")
	fixture.relayToAllDev.SingleChannelControlResponse(1, true, LIGHT_LEVEL_OFF, parseChannelStatus("xx--"))
	fixture.relayToAllDev.ZoneBeastBroadcast([]byte{0}, parseChannelStatus("x---")) // outdated response -- must be ignored
	fixture.Verify(
		"tst -> /devices/zonebeast011c/controls/Channel 1/on: [0] (QoS 1)",
		"driver -> /devices/zonebeast011c/controls/Channel 1: [0] (QoS 1, retained)",
	)

	fixture.driver.Poll()
	fixture.handler.Verify("03/fe (type fffe) -> 01/1c: <ReadTemperatureValues Celsius>")
	fixture.relayToAllDev.ReadTemperatureValuesResponse(true, []int8{22})
	fixture.Verify(
		"driver -> /devices/zonebeast011c/controls/Temp 1/meta/type: [temperature] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Temp 1/meta/readonly: [1] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Temp 1/meta/order: [5] (QoS 1, retained)",
		"driver -> /devices/zonebeast011c/controls/Temp 1: [22] (QoS 1, retained)",
	)

	fixture.driver.Poll()
	fixture.handler.Verify("03/fe (type fffe) -> 01/1c: <ReadTemperatureValues Celsius>")
	fixture.relayToAllDev.ReadTemperatureValuesResponse(true, []int8{-2})
	fixture.Verify(
		"driver -> /devices/zonebeast011c/controls/Temp 1: [-2] (QoS 1, retained)",
	)
}

func TestSmartbusDriverZoneBeastCommandQueue(t *testing.T) {
	fixture := newZoneBeastFixture(t, true)
	defer fixture.tearDown()

	fixture.client.Publish(
		wbgo.MQTTMessage{"/devices/zonebeast011c/controls/Channel 2/on", "1", 1, false})
	fixture.Verify(
		"tst -> /devices/zonebeast011c/controls/Channel 2/on: [1] (QoS 1)",
	)
	fixture.handler.Verify(
		"03/fe (type fffe) -> 01/1c: <SingleChannelControlCommand 2/100/0>")
	fixture.Verify(
		"new fake timer: 1, 1000",
	)

	fixture.FireTimer(1, fixture.AdvanceTime(1000)) // oops, timeout!
	fixture.Verify("timer.fire(): 1")
	fixture.handler.Verify(
		"03/fe (type fffe) -> 01/1c: <SingleChannelControlCommand 2/100/0>")
	fixture.Verify(
		"new fake timer: 2, 1000",
	)
	wbgo.EnsureGotWarnings(t)

	// note that SingleChannelControlResponse carries pre-command channel status
	fixture.relayToAllDev.SingleChannelControlResponse(2, true, LIGHT_LEVEL_ON, parseChannelStatus("x---"))
	fixture.VerifyUnordered(
		"timer.Stop(): 2",
		"driver -> /devices/zonebeast011c/controls/Channel 2: [1] (QoS 1, retained)",
	)
}

// TBD: outdated ZoneBeastBroadcast messages still arrive sometimes, need to fix this
