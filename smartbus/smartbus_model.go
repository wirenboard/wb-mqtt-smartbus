package smartbus

import (
	"fmt"
	"github.com/contactless/wbgo"
	"strconv"
	"strings"
	"time"
)

const (
	// FIXME: make these configurable?
	NUM_VIRTUAL_RELAYS  = 15
	REQUEST_QUEUE_SIZE  = 16
	REQUEST_NUM_RETRIES = 10
	REQUEST_TIMEOUT     = 1000 * time.Millisecond
)

type Request struct {
	name           string
	expectedOpcode uint16
	thunk          func()
}

func newRequest(name string, expectedResponse Message, thunk func()) *Request {
	return &Request{name, expectedResponse.Opcode(), thunk}
}

func (request *Request) Run() {
	request.thunk()
}

func (request *Request) IsResponse(msg Message) bool {
	return msg.Opcode() == request.expectedOpcode
}

func (request *Request) Name() string {
	return request.name
}

type Connector func() (SmartbusIO, error)

type RealDeviceModel interface {
	wbgo.LocalDeviceModel
	Type() uint16
	Poll()
}

type DeviceConstructor func(model *SmartbusModel, smartDev *SmartbusDevice) RealDeviceModel

var smartbusDeviceModelTypes map[uint16]DeviceConstructor = make(map[uint16]DeviceConstructor)

func RegisterDeviceModelType(construct DeviceConstructor) {
	smartbusDeviceModelTypes[construct(nil, nil).Type()] = construct
}

type VirtualRelayDevice struct {
	wbgo.DeviceBase
	channelStatus [NUM_VIRTUAL_RELAYS]bool
}

func (dm *VirtualRelayDevice) Publish() {
	for i, status := range dm.channelStatus {
		v := "0"
		if status {
			v = "1"
		}
		controlName := fmt.Sprintf("VirtualRelay%d", i+1)
		dm.Observer.OnNewControl(dm, controlName, "switch", v, true, -1, true)
	}
}

func (dm *VirtualRelayDevice) SetRelayOn(channelNo int, on bool) {
	if channelNo < 1 || channelNo > NUM_VIRTUAL_RELAYS {
		wbgo.Warn.Printf("invalid virtual relay channel %d", channelNo)
		return
	}
	if dm.channelStatus[channelNo-1] == on {
		return
	}
	dm.channelStatus[channelNo-1] = on
	v := "0"
	if on {
		v = "1"
	}
	controlName := fmt.Sprintf("VirtualRelay%d", channelNo)
	dm.Observer.OnValue(dm, controlName, v)
}

func (dm *VirtualRelayDevice) RelayStatus() []bool {
	return dm.channelStatus[:]
}

func (dm *VirtualRelayDevice) AcceptValue(name, value string) {
	// FIXME: support retained values for virtual relays
}

func (dm *VirtualRelayDevice) AcceptOnValue(name, value string) bool {
	// virtual relays cannot be changed
	return false
}

func (dm *VirtualRelayDevice) IsVirtual() bool {
	return true
}

func NewVirtualRelayDevice() *VirtualRelayDevice {
	r := &VirtualRelayDevice{}
	r.DevName = "sbusvrelay"
	r.DevTitle = "Smartbus Virtual Relays"
	return r
}

type SmartbusModel struct {
	wbgo.ModelBase
	queue         *MessageQueue
	connector     Connector
	deviceMap     map[uint16]RealDeviceModel
	subnetID      uint8
	deviceID      uint8
	deviceType    uint16
	ep            *SmartbusEndpoint
	virtualRelays *VirtualRelayDevice
	broadcastDev  *SmartbusDevice
	timerFunc     TimerFunc
}

func NewSmartbusModel(connector Connector, subnetID uint8,
	deviceID uint8, deviceType uint16, timerFunc TimerFunc) (model *SmartbusModel) {
	model = &SmartbusModel{
		queue: NewMessageQueue(
			timerFunc, REQUEST_TIMEOUT, REQUEST_NUM_RETRIES, REQUEST_QUEUE_SIZE),
		connector:     connector,
		subnetID:      subnetID,
		deviceID:      deviceID,
		deviceType:    deviceType,
		deviceMap:     make(map[uint16]RealDeviceModel),
		virtualRelays: NewVirtualRelayDevice(),
		timerFunc:     timerFunc,
	}
	return
}

func (model *SmartbusModel) Start() error {
	smartbusIO, err := model.connector()
	if err != nil {
		return err
	}
	conn := NewSmartbusConnection(smartbusIO)
	model.ep = conn.MakeSmartbusEndpoint(model.subnetID, model.deviceID, model.deviceType)
	model.ep.Observe(model)
	model.ep.Observe(NewMessageDumper("MESSAGE FOR US"))
	model.ep.AddInputSniffer(NewMessageDumper("NOT FOR US"))
	model.ep.AddOutputSniffer(NewMessageDumper("OUTGOING"))
	model.broadcastDev = model.ep.GetBroadcastDevice()
	model.Observer.OnNewDevice(model.virtualRelays)
	model.virtualRelays.Publish()
	model.queue.Start()
	model.broadcastDev.ReadMACAddress() // discover devices
	return err
}

func (model *SmartbusModel) Stop() {
	model.queue.Stop()
}

func (model *SmartbusModel) Poll() {
	for _, dev := range model.deviceMap {
		dev.Poll()
	}
}

func (model *SmartbusModel) enqueueRequest(name string, expectedResponse Message, thunk func()) {
	model.queue.Enqueue(newRequest(name, expectedResponse, thunk))
}

func (model *SmartbusModel) ensureDevice(header *MessageHeader) RealDeviceModel {
	deviceKey := (uint16(header.OrigSubnetID) << 8) + uint16(header.OrigDeviceID)
	var dev, found = model.deviceMap[deviceKey]
	if found {
		return dev
	}

	construct, found := smartbusDeviceModelTypes[header.OrigDeviceType]
	if !found {
		wbgo.Debug.Printf("unrecognized device type %04x @ %02x:%02x",
			header.OrigDeviceType, header.OrigSubnetID, header.OrigDeviceID)
		return nil
	}

	smartDev := model.ep.GetSmartbusDevice(header.OrigSubnetID, header.OrigDeviceID)
	dev = construct(model, smartDev)
	model.deviceMap[deviceKey] = dev
	wbgo.Debug.Printf("NEW DEVICE: %#v (name: %v)\n", dev, dev.Name())
	model.Observer.OnNewDevice(dev)
	return dev
}

func (model *SmartbusModel) OnAnything(msg Message, header *MessageHeader) {
	model.Observer.CallSync(func() {
		dev := model.ensureDevice(header)
		if dev != nil {
			wbgo.Visit(dev, msg, "On")
		}
	})
}

func (model *SmartbusModel) SetVirtualRelayOn(channelNo int, on bool) {
	model.virtualRelays.SetRelayOn(channelNo, on)
}

func (model *SmartbusModel) VirtualRelayStatus() []bool {
	return model.virtualRelays.RelayStatus()
}

type DeviceModelBase struct {
	nameBase  string
	titleBase string
	model     *SmartbusModel
	smartDev  *SmartbusDevice
	Observer  wbgo.DeviceObserver
}

func (dm *DeviceModelBase) Name() string {
	return fmt.Sprintf("%s%02x%02x", dm.nameBase, dm.smartDev.SubnetID, dm.smartDev.DeviceID)
}

func (dm *DeviceModelBase) Title() string {
	return fmt.Sprintf("%s %02x:%02x", dm.titleBase, dm.smartDev.SubnetID, dm.smartDev.DeviceID)
}

func (dev *DeviceModelBase) Observe(observer wbgo.DeviceObserver) {
	dev.Observer = observer
}

func (dev *DeviceModelBase) AcceptValue(name, value string) {
	// ignore retained values
}

func (dev *DeviceModelBase) OnReadMACAddressResponse(msg *ReadMACAddressResponse) {
	wbgo.Debug.Printf("Got MAC address query response from %s (%s)", dev.Name(), dev.Title())
}

func (dev *DeviceModelBase) IsVirtual() bool { return false }

type ZoneBeastDeviceModel struct {
	DeviceModelBase
	channelStatus []bool
	skipBroadcast bool
	numTemps      int
}

func NewZoneBeastDeviceModel(model *SmartbusModel, smartDev *SmartbusDevice) RealDeviceModel {
	return &ZoneBeastDeviceModel{
		DeviceModelBase{
			nameBase:  "zonebeast",
			titleBase: "Zone Beast",
			model:     model,
			smartDev:  smartDev,
		},
		make([]bool, 0, 100),
		false,
		0,
	}
}

func (dm *ZoneBeastDeviceModel) Type() uint16 { return 0x139c }

func (dm *ZoneBeastDeviceModel) Poll() {
	// no queueing here because polling is periodic
	dm.smartDev.ReadTemperatureValues(true) // FIXME: Celsius is hardcoded here
}

func (dm *ZoneBeastDeviceModel) AcceptOnValue(name, value string) bool {
	wbgo.Debug.Printf("ZoneBeastDeviceModel.AcceptOnValue(%v, %v)", name, value)
	channelNo, err := strconv.Atoi(strings.TrimPrefix(name, "Channel "))
	if err != nil {
		wbgo.Warn.Printf("bad channel name: %s", name)
		return false
	}
	level := uint8(LIGHT_LEVEL_OFF)
	if value == "1" {
		level = LIGHT_LEVEL_ON
	}

	dm.model.enqueueRequest(
		"SingleChannelControl", &SingleChannelControlResponse{},
		func() {
			dm.smartDev.SingleChannelControl(uint8(channelNo), level, 0)
		})

	// No need to echo the value back.
	// It will be echoed after the device response
	return false
}

func (dm *ZoneBeastDeviceModel) OnSingleChannelControlResponse(msg *SingleChannelControlResponse) {
	dm.model.queue.HandleReceivedMessage(msg)
	if !msg.Success {
		wbgo.Error.Printf("ERROR: unsuccessful SingleChannelControlCommand")
		return
	}

	dm.updateSingleChannel(int(msg.ChannelNo-1), msg.Level != 0)
	// ZoneBeast may send an outdated broadcast after SingleChannelControlResponse (?)
	dm.skipBroadcast = true
}

func (dm *ZoneBeastDeviceModel) OnZoneBeastBroadcast(msg *ZoneBeastBroadcast) {
	if !dm.skipBroadcast {
		dm.updateChannelStatus(msg.ChannelStatus)
	}
	dm.skipBroadcast = false
}

func (dm *ZoneBeastDeviceModel) OnReadTemperatureValuesResponse(msg *ReadTemperatureValuesResponse) {
	// if it's not using Celsius, it's not for us
	if msg.UseCelsius {
		for i, v := range msg.Values {
			dm.updateTemperatureValue(i+1, v)
		}
	}
}

func (dm *ZoneBeastDeviceModel) updateSingleChannel(n int, isOn bool) {
	if n >= len(dm.channelStatus) {
		wbgo.Error.Printf("SmartbusModelDevice.updateSingleChannel(): bad channel number: %d", n)
		return
	}

	if dm.channelStatus[n] == isOn {
		return
	}

	dm.channelStatus[n] = isOn
	v := "0"
	if isOn {
		v = "1"
	}
	dm.Observer.OnValue(dm, fmt.Sprintf("Channel %d", n+1), v)
}

func (dm *ZoneBeastDeviceModel) updateChannelStatus(channelStatus []bool) {
	updateCount := len(dm.channelStatus)
	if updateCount > len(channelStatus) {
		updateCount = len(channelStatus)
	}

	for i := 0; i < updateCount; i++ {
		dm.updateSingleChannel(i, channelStatus[i])
	}

	for i := updateCount; i < len(channelStatus); i++ {
		dm.channelStatus = append(dm.channelStatus, channelStatus[i])
		v := "0"
		if dm.channelStatus[i] {
			v = "1"
		}
		controlName := fmt.Sprintf("Channel %d", i+1)
		dm.Observer.OnNewControl(dm, controlName, "switch", v, false, -1, true)
	}
}

func (dm *ZoneBeastDeviceModel) updateTemperatureValue(n int, value int8) {
	// note that this function isn't supposed to be called for some n > 1
	// without being called first for n-1
	controlName := fmt.Sprintf("Temp %d", n)
	valueStr := strconv.Itoa(int(value))
	if n > dm.numTemps {
		dm.Observer.OnNewControl(dm, controlName, "temperature", valueStr, true, -1, true)
		dm.numTemps = n
	} else {
		dm.Observer.OnValue(dm, controlName, valueStr)
	}
}

type DDPDeviceModel struct {
	DeviceModelBase
	buttonAssignmentReceived  []bool
	buttonAssignment          []int
	isNew                     bool
	pendingAssignmentButtonNo int
	pendingAssignment         int
}

func NewDDPDeviceModel(model *SmartbusModel, smartDev *SmartbusDevice) RealDeviceModel {
	return &DDPDeviceModel{
		DeviceModelBase{
			nameBase:  "ddp",
			titleBase: "DDP",
			model:     model,
			smartDev:  smartDev,
		},
		make([]bool, PANEL_BUTTON_COUNT),
		make([]int, PANEL_BUTTON_COUNT),
		true,
		-1,
		-1,
	}
}

func ddpControlName(buttonNo uint8) string {
	return fmt.Sprintf("Page%dButton%d",
		(buttonNo-1)/4+1,
		(buttonNo-1)%4+1)
}

func (dm *DDPDeviceModel) Type() uint16 { return 0x0095 }

func (dm *DDPDeviceModel) Poll() {}

func (dm *DDPDeviceModel) OnReadMACAddressResponse(msg *ReadMACAddressResponse) {
	// NOTE: something like this can be used for button 'learning':
	// dm.smartDev.QueryModulesResponse(QUERY_MODULES_DEV_RELAY, 0x08)
	dm.queryButtons()
}

func (dm *DDPDeviceModel) OnQueryModules(msg *QueryModules) {
	// for the case when the DDP was plugged in
	// after driver init
	dm.queryButtons()
}

func (dm *DDPDeviceModel) queryButtons() {
	if dm.isNew {
		dm.isNew = false
		dm.queryButton(1)
	}
}

func (dm *DDPDeviceModel) queryButton(n uint8) {
	wbgo.Debug.Printf("queryButton(): %d", n)
	dm.model.enqueueRequest(
		"QueryPanelButtonAssignment",
		&QueryPanelButtonAssignmentResponse{},
		func() {
			wbgo.Debug.Printf("queryButton() thunk: %d", n)
			dm.smartDev.QueryPanelButtonAssignment(n, 1)
		})
}

func (dm *DDPDeviceModel) OnQueryPanelButtonAssignmentResponse(msg *QueryPanelButtonAssignmentResponse) {
	dm.model.queue.HandleReceivedMessage(msg)
	// FunctionNo = 1 because we're only querying the first function
	// in the list currently (multiple functions may be needed for CombinationOn mode etc.)
	if msg.ButtonNo == 0 || msg.ButtonNo > PANEL_BUTTON_COUNT || msg.FunctionNo != 1 {
		wbgo.Error.Printf("bad button/fn number: %d/%d", msg.ButtonNo, msg.FunctionNo)
	}

	v := -1
	if msg.Command == BUTTON_COMMAND_SINGLE_CHANNEL_LIGHTING_CONTROL &&
		msg.CommandSubnetID == dm.model.subnetID &&
		msg.CommandDeviceID == dm.model.deviceID {
		v = int(msg.ChannelNo)
	}
	dm.buttonAssignment[msg.ButtonNo-1] = v

	controlName := ddpControlName(msg.ButtonNo)

	if dm.buttonAssignmentReceived[msg.ButtonNo-1] {
		dm.Observer.OnValue(dm, controlName, strconv.Itoa(v))
	} else {
		dm.buttonAssignmentReceived[msg.ButtonNo-1] = true
		dm.Observer.OnNewControl(dm, controlName, "text", strconv.Itoa(v), false, -1, true)
	}

	// TBD: this is not quite correct, should wait w/timeout etc.
	if msg.ButtonNo < PANEL_BUTTON_COUNT {
		dm.queryButton(msg.ButtonNo + 1)
	}
}

func (dm *DDPDeviceModel) OnSetPanelButtonModesResponse(msg *SetPanelButtonModesResponse) {
	// FIXME
	dm.model.queue.HandleReceivedMessage(msg)
	if dm.pendingAssignmentButtonNo <= 0 {
		wbgo.Error.Printf("SetPanelButtonModesResponse without pending assignment")
	}

	dm.model.enqueueRequest(
		"AssignPanelButton", &AssignPanelButtonResponse{}, func() {
			dm.smartDev.AssignPanelButton(
				uint8(dm.pendingAssignmentButtonNo),
				1,
				BUTTON_COMMAND_SINGLE_CHANNEL_LIGHTING_CONTROL,
				dm.model.subnetID,
				dm.model.deviceID,
				uint8(dm.pendingAssignment),
				100,
				0)
		})
}

func (dm *DDPDeviceModel) OnAssignPanelButtonResponse(msg *AssignPanelButtonResponse) {
	dm.model.queue.HandleReceivedMessage(msg)
	if dm.pendingAssignmentButtonNo >= 0 &&
		int(msg.ButtonNo) == dm.pendingAssignmentButtonNo &&
		msg.FunctionNo == 1 {
		dm.Observer.OnValue(dm, ddpControlName(msg.ButtonNo),
			strconv.Itoa(dm.pendingAssignment))
	} else {
		wbgo.Error.Printf("mismatched AssignPanelButtonResponse: %v/%v (pending %d)",
			msg.ButtonNo, msg.FunctionNo, dm.pendingAssignmentButtonNo)
	}
	// FIXME: reset these upon failed command (all retries failed)
	dm.pendingAssignmentButtonNo = -1
	dm.pendingAssignment = -1
}

func (dm *DDPDeviceModel) OnSingleChannelControlCommand(msg *SingleChannelControlCommand) {
	dm.model.SetVirtualRelayOn(int(msg.ChannelNo), msg.Level > 0)
	// Note that we can't guarantee here that the response reaches
	// the device, but we can't do anything about it here
	dm.smartDev.SingleChannelControlResponse(msg.ChannelNo, true, msg.Level,
		dm.model.VirtualRelayStatus())
}

func (dm *DDPDeviceModel) AcceptOnValue(name, value string) bool {
	// FIXME
	if dm.pendingAssignmentButtonNo > 0 {
		wbgo.Error.Printf("button assignment queueing not implemented yet!")
	}

	s1 := strings.TrimPrefix(name, "Page")
	idx := strings.Index(s1, "Button")

	pageNo, err := strconv.Atoi(s1[:idx])
	if err != nil {
		wbgo.Error.Printf("bad button param: %s", name)
		return false
	}

	pageButtonNo, err := strconv.Atoi(s1[idx+6:])
	if err != nil {
		wbgo.Error.Printf("bad button param: %s", name)
		return false
	}

	buttonNo := (pageNo-1)*4 + pageButtonNo

	newAssignment, err := strconv.Atoi(value)
	if err != nil || newAssignment <= 0 || newAssignment > NUM_VIRTUAL_RELAYS {
		wbgo.Error.Printf("bad button assignment value: %s", value)
		return false
	}

	for _, isReceived := range dm.buttonAssignmentReceived {
		if !isReceived {
			// TBD: fix this
			wbgo.Error.Printf("cannot assign button: DDP device data not ready yet")
			return false
		}
	}

	var modes [PANEL_BUTTON_COUNT]string
	for i, assignment := range dm.buttonAssignment {
		if buttonNo == i+1 {
			assignment = newAssignment
			dm.buttonAssignment[i] = newAssignment
		}
		if assignment <= 0 || assignment > NUM_VIRTUAL_RELAYS {
			modes[i] = "Invalid"
		} else {
			modes[i] = "SingleOnOff"
		}
	}

	dm.model.enqueueRequest(
		"SetPanelButtonModes",
		&SetPanelButtonModesResponse{},
		func() {
			dm.smartDev.SetPanelButtonModes(modes)
		})

	dm.pendingAssignmentButtonNo = buttonNo
	dm.pendingAssignment = newAssignment

	return false
}

func init() {
	RegisterDeviceModelType(NewZoneBeastDeviceModel)
	RegisterDeviceModelType(NewDDPDeviceModel)
}
