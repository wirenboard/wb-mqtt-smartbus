GOM=gom
.PHONY: all clean

all: wb-mqtt-smartbus
clean:

prepare:
	go get -u github.com/mattn/gom
	PATH=$(HOME)/progs/go/bin:$(PATH) GOARM=5 GOARCH=arm GOOS=linux \
	  CC_FOR_TARGET=arm-linux-gnueabi-gcc CGO_ENABLED=1 $(GOM) install

clean:
	rm -f wb-mqtt-smartbus

wb-mqtt-smartbus: smartbus_driver.go smartbus/*.go
	PATH=$(HOME)/progs/go/bin:$(PATH) GOARM=5 GOARCH=arm GOOS=linux \
	  CC_FOR_TARGET=arm-linux-gnueabi-gcc CGO_ENABLED=1 $(GOM) build

install:
	mkdir -p $(DESTDIR)/usr/bin/ $(DESTDIR)/etc/init.d/
	install -m 0755 wb-mqtt-smartbus $(DESTDIR)/usr/bin/
	install -m 0755  initscripts/wb-mqtt-smartbus $(DESTDIR)/etc/init.d/wb-mqtt-smartbus
