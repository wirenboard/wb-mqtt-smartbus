GOM=gom
.PHONY: all prepare clean

GOPATH := $(HOME)/go
PATH := $(HOME)/progs/go/bin:$(GOPATH)/bin:$(PATH)

DEB_TARGET_ARCH ?= armel

ifeq ($(DEB_TARGET_ARCH),armel)
GO_ENV := GOARCH=arm GOARM=5 CC_FOR_TARGET=arm-linux-gnueabi-gcc CGO_ENABLED=1
endif
ifeq ($(DEB_TARGET_ARCH),amd64)
GO_ENV := GOARCH=amd64 CC=x86_64-linux-gnu-gcc
endif
ifeq ($(DEB_TARGET_ARCH),i386)
GO_ENV := GOARCH=386 CC=i586-linux-gnu-gcc
endif

all: wb-mqtt-smartbus

prepare:
	go get -u github.com/mattn/gom

clean:
	rm -f wb-mqtt-smartbus

wb-mqtt-smartbus: smartbus_driver.go smartbus/*.go
	$(GO_ENV) $(GOM) install
	$(GO_ENV) $(GOM) build

install:
	mkdir -p $(DESTDIR)/usr/bin/ $(DESTDIR)/etc/init.d/
	install -m 0755 wb-mqtt-smartbus $(DESTDIR)/usr/bin/
	install -m 0755  initscripts/wb-mqtt-smartbus $(DESTDIR)/etc/init.d/wb-mqtt-smartbus
