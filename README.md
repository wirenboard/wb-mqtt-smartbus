wb-mqtt-smartbus
================

MQTT-драйвер [Smart-Bus](http://smarthomebus.com/) для Wiren Board.

Сборка пакета для Wiren Board (например, на Ubuntu 14.04):
```
sudo apt-get install golang-go build-essential fakeroot dpkg-dev \
  debhelper pkg-config binutils-arm-linux-gnueabi git mercurial
git clone https://github.com/contactless/wb-mqtt-smartbus.git
cd wb-mqtt-smartbus
export GOPATH=~/go
mkdir -p $GOPATH
export PATH=$PATH:$GOPATH/bin
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

