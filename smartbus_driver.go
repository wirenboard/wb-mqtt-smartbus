package main

import (
	"github.com/contactless/wb-mqtt-smartbus/smartbus"
	"flag"
	"github.com/contactless/wbgo"
	"time"
)

func main() {
	serial := flag.String("serial", "/dev/ttyNSC1", "serial port address (/dev/... or host:port)")
	broker := flag.String("broker", "tcp://localhost:1883", "MQTT broker url")
	gw := flag.Bool("gw", false, "Provide UDP gateway")
	debug := flag.Bool("debug", false, "Enable debugging")
	flag.Parse()
	if *debug {
		wbgo.SetDebuggingEnabled(true)
	}
	if driver, err := smartbus.NewSmartbusTCPDriver(*serial, *broker, *gw); err != nil {
		panic(err)
	} else {
		if err := driver.Start(); err != nil {
			panic(err)
		}
		for {
			time.Sleep(1 * time.Second)
		}
	}
}
