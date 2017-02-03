.PHONY: all prepare clean

DEB_TARGET_ARCH ?= armel

ifeq ($(DEB_TARGET_ARCH),armel)
GO_ENV := GOARCH=arm GOARM=5 CC_FOR_TARGET=arm-linux-gnueabi-gcc CC=$$CC_FOR_TARGET CGO_ENABLED=1
endif
ifeq ($(DEB_TARGET_ARCH),armhf)
GO_ENV := GOARCH=arm GOARM=6 CC_FOR_TARGET=arm-linux-gnueabihf-gcc CC=$$CC_FOR_TARGET CGO_ENABLED=1
endif
ifeq ($(DEB_TARGET_ARCH),amd64)
GO_ENV := GOARCH=amd64 CC=x86_64-linux-gnu-gcc
endif
ifeq ($(DEB_TARGET_ARCH),i386)
GO_ENV := GOARCH=386 CC=i586-linux-gnu-gcc
endif

all: wb-mqtt-smartbus

clean:
	rm -f wb-mqtt-smartbus

amd64:
	$(MAKE) DEB_TARGET_ARCH=amd64

wb-mqtt-smartbus: smartbus_driver.go smartbus/*.go
	$(GO_ENV) glide install
	$(GO_ENV) go build

install:
	mkdir -p $(DESTDIR)/usr/bin/ $(DESTDIR)/etc/init.d/
	install -m 0755 wb-mqtt-smartbus $(DESTDIR)/usr/bin/
	install -m 0755 initscripts/wb-mqtt-smartbus $(DESTDIR)/etc/init.d/wb-mqtt-smartbus
