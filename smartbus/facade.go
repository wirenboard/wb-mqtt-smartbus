package smartbus

import (
	"errors"
	"github.com/contactless/wbgo"
	"github.com/goburrow/serial"
	"io"
	"net"
	"strings"
	"time"
)

// FIXME
const (
	DRIVER_SUBNET      = 0x01
	DRIVER_DEVICE_ID   = 0x99
	DRIVER_DEVICE_TYPE = 0x1234
	DRIVER_CLIENT_ID   = "smartbus"
)

func createStreamIO(stream io.ReadWriteCloser, provideUdpGateway bool) (SmartbusIO, error) {
	if !provideUdpGateway {
		return NewStreamIO(stream, nil), nil
	}
	rawUdpReadCh := make(chan []byte)
	rawSerialReadCh := make(chan []byte)
	wbgo.Debug.Println("using UDP gateway mode")
	dgramIO, err := NewDatagramIO(rawUdpReadCh)
	if err != nil {
		return nil, err
	}
	streamIO := NewStreamIO(stream, rawSerialReadCh)
	dgramIO.Start()
	go func() {
		for frame := range rawUdpReadCh {
			streamIO.SendRaw(frame)
		}
	}()
	go func() {
		for frame := range rawSerialReadCh {
			dgramIO.SendRaw(frame)
		}
	}()
	return streamIO, nil
}

func connect(serialAddress string, provideUdpGateway bool) (SmartbusIO, error) {
	switch {
	case strings.HasPrefix(serialAddress, "/"):
		if port, err := serial.Open(&serial.Config{
			Address:  serialAddress,
			BaudRate: 9600,
			DataBits: 8,
			StopBits: 2,
			Parity:   "E",
		}); err != nil {
			return nil, err
		} else {
			return createStreamIO(port, provideUdpGateway)
		}
	case serialAddress == "udp":
		if provideUdpGateway {
			return nil, errors.New("cannot provide UDP gw in udp device access mode")
		}
		if dgramIO, err := NewDatagramIO(nil); err != nil {
			return nil, err
		} else {
			return dgramIO, nil
		}
	case strings.HasPrefix(serialAddress, "tcp://"):
		if conn, err := net.Dial("tcp", serialAddress[6:]); err != nil {
			return nil, err
		} else {
			return createStreamIO(conn, provideUdpGateway)
		}
	}

	if conn, err := net.Dial("tcp", serialAddress); err != nil {
		return nil, err
	} else {
		return NewStreamIO(conn, nil), nil
	}
}

func NewSmartbusTCPDriver(serialAddress, brokerAddress string, provideUdpGateway bool) (*wbgo.Driver, error) {
	model := NewSmartbusModel(func() (SmartbusIO, error) {
		return connect(serialAddress, provideUdpGateway)
	}, DRIVER_SUBNET, DRIVER_DEVICE_ID, DRIVER_DEVICE_TYPE, func(d time.Duration) wbgo.Timer {
		return wbgo.NewRealTimer(d)
	})
	driver := wbgo.NewDriver(model, wbgo.NewPahoMQTTClient(brokerAddress, DRIVER_CLIENT_ID, false))
	return driver, nil
}
