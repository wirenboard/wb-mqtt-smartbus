package smartbus

// TBD: should prefix all hex vals with 0x

import (
	"fmt"
	"github.com/contactless/wbgo"
	"strconv"
	"strings"
)

type MessageFormatter struct {
	AddMessage func(format string, args ...interface{})
}

func formatChannelStatus(status []bool) (result string) {
	for _, s := range status {
		if s {
			result += "x"
		} else {
			result += "-"
		}
	}
	return
}

func (f *MessageFormatter) log(header *MessageHeader, format string, args ...interface{}) {
	f.AddMessage("%02x/%02x (type %04x) -> %02x/%02x: %s",
		header.OrigSubnetID,
		header.OrigDeviceID,
		header.OrigDeviceType,
		header.TargetSubnetID,
		header.TargetDeviceID,
		fmt.Sprintf(format, args...))
}

func (f *MessageFormatter) OnSingleChannelControlCommand(msg *SingleChannelControlCommand, hdr *MessageHeader) {
	f.log(hdr,
		"<SingleChannelControlCommand %v/%v/%v>",
		msg.ChannelNo,
		msg.Level,
		msg.Duration)
}

func (f *MessageFormatter) OnSingleChannelControlResponse(msg *SingleChannelControlResponse,
	hdr *MessageHeader) {
	f.log(hdr,
		"<SingleChannelControlResponse %v/%v/%v/%s>",
		msg.ChannelNo,
		msg.Success,
		msg.Level,
		formatChannelStatus(msg.ChannelStatus))
}

func (f *MessageFormatter) OnZoneBeastBroadcast(msg *ZoneBeastBroadcast,
	hdr *MessageHeader) {
	f.log(hdr,
		"<ZoneBeastBroadcast %v/%s>",
		msg.ZoneStatus,
		formatChannelStatus(msg.ChannelStatus))
}

func (f *MessageFormatter) OnSceneControl(msg *SceneControl, hdr *MessageHeader) {
	f.log(hdr,
		"<SceneControl %02x/%02x>",
		msg.ZoneNo,
		msg.SceneNo)
}

func (f *MessageFormatter) OnSceneControlResponse(msg *SceneControlResponse, hdr *MessageHeader) {
	f.log(hdr,
		"<SceneControlResponse %02x/%02x/%s>",
		msg.ZoneNo,
		msg.SceneNo,
		formatChannelStatus(msg.ChannelStatus))
}

func (f *MessageFormatter) OnQueryModules(msg *QueryModules, hdr *MessageHeader) {
	f.log(hdr, "<QueryModules>")
}

func (f *MessageFormatter) OnQueryModulesResponse(msg *QueryModulesResponse,
	hdr *MessageHeader) {
	f.log(hdr, "<QueryModulesResponse %02x/%02x/%02x/%v/%02x/%02x>",
		msg.ControlledDeviceSubnetID,
		msg.ControlledDeviceID,
		msg.DeviceCategory,
		msg.ChannelNo,
		msg.HVACSubnetID,
		msg.HVACDeviceID)
}

var panelControlTypes map[uint8]string = map[uint8]string{
	0x00: "Invalid",
	0x01: "IR Receiver",
	0x02: "Button Lock",
	0x03: "AC On/Off",
	0x04: "Cooling Set Point",
	0x05: "Fan Speed",
	0x06: "AC Mode",
	0x07: "Heat Set Point",
	0x08: "Auto Set Point",
	0x16: "Go To Page",
}

func (f *MessageFormatter) OnPanelControlResponse(msg *PanelControlResponse,
	hdr *MessageHeader) {
	typeName, found := panelControlTypes[msg.Type]
	if !found {
		typeName = fmt.Sprintf("<unknown type 0x%02x>", msg.Type)
	}
	f.log(hdr, "<PanelControlResponse %s=%v>", typeName, msg.Value)
}

func (f *MessageFormatter) OnQueryChannelStatuses(msg *QueryChannelStatuses,
	hdr *MessageHeader) {
	f.log(hdr, "<QueryChannelStatuses %d>", msg.Index)
}

func (f *MessageFormatter) OnQueryChannelStatusesResponse(msg *QueryChannelStatusesResponse,
	hdr *MessageHeader) {
	f.log(hdr, "<QueryChannelStatusesResponse %v>", msg.ChannelStatus)
}

func (f *MessageFormatter) OnQueryPanelButtonAssignment(msg *QueryPanelButtonAssignment,
	hdr *MessageHeader) {
	f.log(hdr, "<QueryPanelButtonAssignment %v/%v>", msg.ButtonNo, msg.FunctionNo)
}

func (f *MessageFormatter) OnQueryPanelButtonAssignmentResponse(msg *QueryPanelButtonAssignmentResponse, hdr *MessageHeader) {
	f.log(hdr,
		"<QueryPanelButtonAssignmentResponse %v/%v/%02x/%02x/%02x/%v/%v/%v>",
		msg.ButtonNo,
		msg.FunctionNo,
		msg.Command,
		msg.CommandSubnetID,
		msg.CommandDeviceID,
		msg.ChannelNo,
		msg.Level,
		msg.Duration)
}

func (f *MessageFormatter) OnAssignPanelButton(msg *AssignPanelButton, hdr *MessageHeader) {
	f.log(hdr,
		"<AssignPanelButton %v/%v/%02x/%02x/%02x/%v/%v/%v/%v>",
		msg.ButtonNo,
		msg.FunctionNo,
		msg.Command,
		msg.CommandSubnetID,
		msg.CommandDeviceID,
		msg.ChannelNo,
		msg.Level,
		msg.Duration,
		msg.Unknown)
}

func (f *MessageFormatter) OnAssignPanelButtonResponse(msg *AssignPanelButtonResponse,
	hdr *MessageHeader) {
	f.log(hdr, "<AssignPanelButtonResponse %v/%v>", msg.ButtonNo, msg.FunctionNo)
}

func (f *MessageFormatter) OnSetPanelButtonModes(msg *SetPanelButtonModes, hdr *MessageHeader) {
	m := make([]string, len(msg.Modes))
	for i, mode := range msg.Modes {
		m[i] = fmt.Sprintf("%d/%d:%s", i/4+1, i%4+1, mode)
	}
	f.log(hdr, "<SetPanelButtonModes %v>", strings.Join(m, ","))
}

func (f *MessageFormatter) OnSetPanelButtonModesResponse(msg *SetPanelButtonModesResponse,
	hdr *MessageHeader) {
	f.log(hdr, "<SetPanelButtonModesResponse %v>", msg.Success)
}

func (f *MessageFormatter) OnReadMACAddress(msg *ReadMACAddress,
	hdr *MessageHeader) {
	f.log(hdr, "<ReadMACAddress>")
}

func (f *MessageFormatter) OnReadMACAddressResponse(msg *ReadMACAddressResponse,
	hdr *MessageHeader) {
	macParts := make([]string, len(msg.MAC))
	for i, v := range msg.MAC {
		macParts[i] = fmt.Sprintf("%02x", v)
	}
	remarkParts := make([]string, len(msg.Remark))
	for i, v := range msg.Remark {
		remarkParts[i] = fmt.Sprintf("%02x", v)
	}
	f.log(hdr, "<ReadMACAddressResponse %s [%s]>",
		strings.Join(macParts, ":"),
		strings.Join(remarkParts, " "))
}

func (f *MessageFormatter) OnReadTemperatureValues(msg *ReadTemperatureValues, hdr *MessageHeader) {
	unitStr := "Fahrenheit"
	if msg.UseCelsius {
		unitStr = "Celsius"
	}

	f.log(hdr, "<ReadTemperatureValues %s>", unitStr)
}

func (f *MessageFormatter) OnReadTemperatureValuesResponse(msg *ReadTemperatureValuesResponse, hdr *MessageHeader) {
	unitStr := "Fahrenheit"
	if msg.UseCelsius {
		unitStr = "Celsius"
	}

	tempStrs := make([]string, len(msg.Values))
	for i, v := range msg.Values {
		tempStrs[i] = strconv.Itoa(int(v))
	}

	f.log(hdr, "<ReadTemperatureValuesResponse %s %s>", unitStr, strings.Join(tempStrs, ","))
}

func (f *MessageFormatter) OnReadSensorStatus(msg *ReadSensorStatus, hdr *MessageHeader) {
	f.log(hdr, "<ReadSensorStatus>")
}

func (f *MessageFormatter) OnReadSensorStatusResponse(msg *ReadSensorStatusResponse, hdr *MessageHeader) {
	f.log(hdr, "<ReadSensorStatusResponse %v %d/%d/%v/%v/%v>",
		msg.Success, msg.Temperature, msg.Illuminance, msg.Movement,
		msg.DryContact1, msg.DryContact2)
}

func (f *MessageFormatter) OnSensorStatusBroadcast(msg *SensorStatusBroadcast, hdr *MessageHeader) {
	f.log(hdr, "<SensorStatusBroadcast %d/%d/%v/%v/%v>",
		msg.Temperature, msg.Illuminance, msg.Movement,
		msg.DryContact1, msg.DryContact2)
}

type MessageDumper struct {
	MessageFormatter
}

func NewMessageDumper(prefix string) *MessageDumper {
	return &MessageDumper{
		MessageFormatter{
			func(format string, args ...interface{}) {
				s := fmt.Sprintf(format, args...)
				wbgo.Debug.Printf("%s: %s", prefix, s)
			},
		},
	}
}
