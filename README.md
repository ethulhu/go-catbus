<!--
SPDX-FileCopyrightText: 2020 Ethel Morgan

SPDX-License-Identifier: MIT
-->

# Catbus client library for Go

[Catbus](https://ethulhu.co.uk/catbus) is a home automation platform built on MQTT.
This library wraps [Paho MQTT](https://godoc.org/github.com/eclipse/paho.mqtt.golang) for convenience, and some behavior additions.

## Rebroadcast on connect

To announce that a non-observing actuator exists, the actuator will need to broadcast an initial value on connect.
It does so using either a default value, or the last seen value put onto the bus, and will publish it after some delay after connecting.
