GOM=gom
.PHONY: all clean

all: wb-mqtt-smartbus
clean:

prepare:
	go get -u github.com/mattn/gom
	$(GOM) install

clean:
	rm -f wb-mqtt-smartbus

wb-mqtt-smartbus: smartbus_driver.go smartbus/*.go
	GOARM=5 GOARCH=arm GOOS=linux $(GOM) build

install:
	mkdir -p $(DESTDIR)/usr/bin/ $(DESTDIR)/etc/init.d/
	install -m 0755 wb-mqtt-smartbus $(DESTDIR)/usr/bin/
	install -m 0755  initscripts/wb-mqtt-smartbus $(DESTDIR)/etc/init.d/wb-mqtt-smartbus
