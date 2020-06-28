// SPDX-FileCopyrightText: 2020 Ethel Morgan
//
// SPDX-License-Identifier: MIT

package catbus

import (
	"fmt"
	"math/rand"
	"os"
	"path"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type (
	client struct {
		mqtt mqtt.Client

		payloadByTopicMu sync.Mutex
		payloadByTopic   map[string]string

		onconnectTimerByTopicMu sync.Mutex
		onconnectTimerByTopic   map[string]*time.Timer

		onconnectDelay  time.Duration
		onconnectJitter time.Duration
	}

	ClientOptions struct {
		DisconnectHandler func(Client, error)
		ConnectHandler    func(Client)

		// Publish previously seen or default values on connecting after OnconnectDelay ± [0,OnconnectJitter).
		OnconnectDelay  time.Duration
		OnconnectJitter time.Duration

		// DefaultPayloadByTopic are optional values to publish on connect if no prior values are seen.
		// E.g. unless we've been told otherwise, assume a device is off.
		DefaultPayloadByTopic map[string]string
	}
)

const (
	atMostOnce byte = iota
	atLeastOnce
	exactlyOnce
)

const (
	DefaultOnconnectDelay  = 1 * time.Minute
	DefaultOnconnectJitter = 15 * time.Second
)

func NewClient(brokerURI string, options ClientOptions) Client {
	client := &client{
		payloadByTopic:        map[string]string{},
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
	mqttOpts.SetClientID(defaultClientID())
	mqttOpts.SetOnConnectHandler(func(_ mqtt.Client) {
		client.stopAllTimers()
		client.startAllTimers()

		if options.ConnectHandler != nil {
			options.ConnectHandler(client)
		}
	})
	mqttOpts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		client.stopAllTimers()

		if options.DisconnectHandler != nil {
			options.DisconnectHandler(client, err)
		}
	})
	client.mqtt = mqtt.NewClient(mqttOpts)

	return client
}

// Connect connects to the Catbus MQTT broker and blocks forever.
func (c *client) Connect() error {
	if err := c.mqtt.Connect().Error(); err != nil {
		return err
	}
	select {}
}

// Subscribe subscribes to a Catbus MQTT topic.
func (c *client) Subscribe(topic string, f MessageHandler) error {
	return c.mqtt.Subscribe(topic, atLeastOnce, func(_ mqtt.Client, msg mqtt.Message) {
		c.storePayload(msg.Topic(), Retention(msg.Retained()), string(msg.Payload()))

		go f(c, messageFromMQTTMessage(msg))
	}).Error()
}

// Publish publishes to a Catbus MQTT topic.
func (c *client) Publish(topic string, retention Retention, payload string) error {
	c.storePayload(topic, retention, payload)

	return c.mqtt.Publish(topic, atLeastOnce, bool(retention), payload).Error()
}

func (c *client) jitteredOnconnectDelay() time.Duration {
	jitter := time.Duration(rand.Intn(int(c.onconnectJitter)))
	if rand.Intn(2) == 0 {
		return c.onconnectDelay + jitter
	}
	return c.onconnectDelay - jitter
}

func (c *client) storePayload(topic string, retention Retention, payload string) {
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
func (c *client) stopTimer(topic string) {
	c.onconnectTimerByTopicMu.Lock()
	defer c.onconnectTimerByTopicMu.Unlock()

	if timer, ok := c.onconnectTimerByTopic[topic]; ok {
		_ = timer.Stop()
	}
}
func (c *client) stopAllTimers() {
	c.onconnectTimerByTopicMu.Lock()
	defer c.onconnectTimerByTopicMu.Unlock()

	for _, timer := range c.onconnectTimerByTopic {
		_ = timer.Stop()
	}
}
func (c *client) startAllTimers() {
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

func messageFromMQTTMessage(msg mqtt.Message) Message {
	return Message{
		Payload:   string(msg.Payload()),
		Retention: Retention(msg.Retained()),
		Topic:     msg.Topic(),
	}
}

func defaultClientID() string {
	binary := path.Base(os.Args[0])
	hostname, _ := os.Hostname()
	pid := os.Getpid()
	return fmt.Sprintf("%s_%s_%d", binary, hostname, pid)
}
