wb-mqtt-smartbus
================

MQTT-драйвер [Smart-Bus](http://smarthomebus.com/) для Wiren Board.

Сборка go1.4.1 с поддержкой CGo (например, на Ubuntu 14.04):

```
sudo apt-get install -y build-essential fakeroot dpkg-dev \
  debhelper pkg-config binutils-arm-linux-gnueabi git mercurial gcc-arm-linux-gnueabi
mkdir progs && cd progs
git clone https://go.googlesource.com/go
cd go
git checkout go1.4.1
cd src
GOARM=5 GOARCH=arm GOOS=linux CC_FOR_TARGET=arm-linux-gnueabi-gcc CGO_ENABLED=1 ./make.bash
```

Сборка пакета для Wiren Board:
```
cd
git clone https://github.com/contactless/wb-mqtt-smartbus
cd wb-mqtt-smartbus/
export GOPATH=~/go
mkdir -p $GOPATH
export PATH=$HOME/progs/go/bin:$GOPATH/bin:$PATH
make prepare
dpkg-buildpackage -b -aarmel -us -uc
```

На данный момент драйвер осуществляет сканирование шины и
конфигурируется автоматически.
Можно задать порт и опцию `-gw` (ethernet-гейтвей)
в `/etc/default/wb-mqtt-smartbus`:
```
SMARTBUS_OPTIONS="-serial /dev/ttyNSC0 -gw"
```

Приведённая конфигурация соответствует настройкам по умолчанию.

