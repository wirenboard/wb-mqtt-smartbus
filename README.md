wb-mqtt-smartbus
================

MQTT-драйвер [Smart-Bus](http://smarthomebus.com/) для Wiren Board.

Сборка исполняемого файла для arm:

```
wbdev hmake clean && wbdev hmake
```

Сборка исполняемого файла для x86_64:

```
wbdev hmake clean && wbdev hmake amd64
```

Сборка пакета для Wiren Board:
```
wbdev gdeb
```

На данный момент драйвер осуществляет сканирование шины и
конфигурируется автоматически.
Можно задать порт и опцию `-gw` (ethernet-гейтвей)
в `/etc/default/wb-mqtt-smartbus`:
```
SMARTBUS_OPTIONS="-serial /dev/ttyNSC0 -gw"
```

Приведённая конфигурация соответствует настройкам по умолчанию.

