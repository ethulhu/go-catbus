// SPDX-FileCopyrightText: 2020 Ethel Morgan
//
// SPDX-License-Identifier: MIT

// Package catbus is a convenience wrapper around MQTT for use with Catbus.
package catbus

import (
	"math/rand"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type (
	Message        = mqtt.Message
	MessageHandler = func(*Client, Message)

	Client struct {
		mqtt mqtt.Client

		payloadByTopicMu sync.Mutex
		payloadByTopic   map[string][]byte

		onconnectTimerByTopicMu sync.Mutex
		onconnectTimerByTopic   map[string]*time.Timer

		onconnectDelay  time.Duration
		onconnectJitter time.Duration
	}

	ClientOptions struct {
		DisconnectHandler func(*Client, error)
		ConnectHandler    func(*Client)

		// Publish previously seen or default values on connecting after OnconnectDelay ± [0,OnconnectJitter).
		OnconnectDelay  time.Duration
		OnconnectJitter time.Duration

		// DefaultPayloadByTopic are optional values to publish on connect if no prior values are seen.
		// E.g. unless we've been told otherwise, assume a device is off.
		DefaultPayloadByTopic map[string][]byte
	}

	// Retention is whether or not the MQTT broker should retain the message.
	Retention bool
)

const (
	atMostOnce byte = iota
	atLeastOnce
	exactlyOnce
)

const (
	Retain     = Retention(true)
	DontRetain = Retention(false)
)

const (
	DefaultOnconnectDelay  = 1 * time.Minute
	DefaultOnconnectJitter = 15 * time.Second
)

func NewClient(brokerURI string, options ClientOptions) *Client {
	client := &Client{
		payloadByTopic:        map[string][]byte{},
		onconnectTimerByTopic: map[string]*time.Timer{},

		onconnectDelay:  DefaultOnconnectDelay,
		onconnectJitter: DefaultOnconnectJitter,
	}

	if options.OnconnectDelay != 0 {
		client.onconnectDelay = options.OnconnectDelay
	}
	if options.OnconnectJitter != 0 {
		client.onconnectJitter = options.OnconnectJitter
	}
	for topic, payload := range options.DefaultPayloadByTopic {
		client.payloadByTopic[topic] = payload
	}

	mqttOpts := mqtt.NewClientOptions()
	mqttOpts.AddBroker(brokerURI)
	mqttOpts.SetAutoReconnect(true)
	mqttOpts.SetOnConnectHandler(func(c mqtt.Client) {
		client.stopAllTimers()
		client.startAllTimers()

		if options.ConnectHandler != nil {
			options.ConnectHandler(client)
		}
	})
	mqttOpts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		client.stopAllTimers()

		if options.DisconnectHandler != nil {
			options.DisconnectHandler(client, err)
		}
	})
	client.mqtt = mqtt.NewClient(mqttOpts)

	return client
}

// Connect connects to the Catbus MQTT broker and blocks forever.
func (c *Client) Connect() error {
	if err := c.mqtt.Connect().Error(); err != nil {
		return err
	}
	select {}
}

// Subscribe subscribes to a Catbus MQTT topic.
func (c *Client) Subscribe(topic string, f MessageHandler) error {
	return c.mqtt.Subscribe(topic, atLeastOnce, func(_ mqtt.Client, msg mqtt.Message) {
		c.storePayload(msg.Topic(), Retention(msg.Retained()), msg.Payload())

		f(c, msg)
	}).Error()
}

// Publish publishes to a Catbus MQTT topic.
func (c *Client) Publish(topic string, retention Retention, payload []byte) error {
	c.storePayload(topic, retention, payload)

	return c.mqtt.Publish(topic, atLeastOnce, bool(retention), payload).Error()
}

func (c *Client) jitteredOnconnectDelay() time.Duration {
	jitter := time.Duration(rand.Intn(int(c.onconnectJitter)))
	if rand.Intn(2) == 0 {
		return c.onconnectDelay + jitter
	}
	return c.onconnectDelay - jitter
}

func (c *Client) storePayload(topic string, retention Retention, payload []byte) {
	c.payloadByTopicMu.Lock()
	defer c.payloadByTopicMu.Unlock()

	if _, ok := c.payloadByTopic[topic]; !ok && retention == DontRetain {
		// If we don't have a copy, and the sender doesn't want it retained, don't retain it.
		return
	}

	c.stopTimer(topic)

	if len(payload) == 0 {
		delete(c.payloadByTopic, topic)
		return
	}
	c.payloadByTopic[topic] = payload
}
func (c *Client) stopTimer(topic string) {
	c.onconnectTimerByTopicMu.Lock()
	defer c.onconnectTimerByTopicMu.Unlock()

	if timer, ok := c.onconnectTimerByTopic[topic]; ok {
		_ = timer.Stop()
	}
}
func (c *Client) stopAllTimers() {
	c.onconnectTimerByTopicMu.Lock()
	defer c.onconnectTimerByTopicMu.Unlock()

	for _, timer := range c.onconnectTimerByTopic {
		_ = timer.Stop()
	}
}
func (c *Client) startAllTimers() {
	c.payloadByTopicMu.Lock()
	defer c.payloadByTopicMu.Unlock()

	c.onconnectTimerByTopicMu.Lock()
	defer c.onconnectTimerByTopicMu.Unlock()

	for topic := range c.payloadByTopic {
		c.onconnectTimerByTopic[topic] = time.AfterFunc(c.jitteredOnconnectDelay(), func() {
			c.payloadByTopicMu.Lock()
			payload, ok := c.payloadByTopic[topic]
			c.payloadByTopicMu.Unlock()
			if !ok {
				return
			}
			_ = c.Publish(topic, Retain, payload)
		})
	}
}