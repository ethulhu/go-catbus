// SPDX-FileCopyrightText: 2020 Ethel Morgan
//
// SPDX-License-Identifier: MIT

// Package catbus is a convenience wrapper around MQTT for use with Catbus.
package catbus

type (
	MessageHandler func(Client, Message)

	Message struct {
		Payload   string
		Retention Retention
		Topic     string
	}

	Client interface {
		Connect() error
		Subscribe(topic string, f MessageHandler) error
		Publish(topic string, retention Retention, payload string) error
	}

	// Retention is whether or not the MQTT broker should retain the message.
	Retention bool
)

const (
	Retain     = Retention(true)
	DontRetain = Retention(false)
)
