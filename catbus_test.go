// SPDX-FileCopyrightText: 2020 Ethel Morgan
//
// SPDX-License-Identifier: MIT

package catbus

import (
	"fmt"
	"log"
	"reflect"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type (
	message struct {
		retention Retention
		payload   []byte
	}
)

func TestOnConnect(t *testing.T) {
	tests := []struct {
		payloadByTopic map[string][]byte
		subscribe      []string
		receive        map[string]message

		want map[string][]byte
	}{
		{
			payloadByTopic: map[string][]byte{
				"tv/power": []byte("off"),
			},
			want: map[string][]byte{
				"tv/power": []byte("off"),
			},
		},
		{
			payloadByTopic: map[string][]byte{
				"tv/power": []byte("off"),
			},
			subscribe: []string{
				"tv/power",
			},
			receive: map[string]message{
				"tv/power": {Retain, []byte("on")},
			},
			want: map[string][]byte{
				"tv/power": []byte("on"),
			},
		},
		{
			subscribe: []string{
				"tv/power",
			},
			receive: map[string]message{
				"tv/power": {Retain, []byte("on")},
			},
			want: map[string][]byte{
				"tv/power": []byte("on"),
			},
		},
		{
			payloadByTopic: map[string][]byte{
				"tv/power": []byte("off"),
			},
			subscribe: []string{
				"tv/power",
			},
			receive: map[string]message{
				"tv/power": {DontRetain, []byte("on")},
			},
			want: map[string][]byte{
				"tv/power": []byte("on"),
			},
		},
		{
			subscribe: []string{
				"tv/power",
			},
			receive: map[string]message{
				"tv/power": {DontRetain, []byte("on")},
			},
			want: map[string][]byte{},
		},
		{
			payloadByTopic: map[string][]byte{
				"tv/power": []byte("off"),
			},
			subscribe: []string{
				"tv/power",
			},
			receive: map[string]message{
				"tv/power": {DontRetain, []byte{}},
			},
			want: map[string][]byte{},
		},
	}

	for i, tt := range tests {
		fakeMQTT := &fakeMQTT{
			callbackByTopic: map[string]mqtt.MessageHandler{},
			payloadByTopic:  map[string][]byte{},
		}

		catbus := &Client{
			mqtt:                  fakeMQTT,
			payloadByTopic:        map[string][]byte{},
			onconnectTimerByTopic: map[string]*time.Timer{},
			onconnectDelay:        1 * time.Millisecond,
			onconnectJitter:       1,
		}
		if tt.payloadByTopic != nil {
			catbus.payloadByTopic = tt.payloadByTopic
		}

		for _, topic := range tt.subscribe {
			catbus.Subscribe(topic, func(_ *Client, _ Message) {})
		}
		for topic, message := range tt.receive {
			fakeMQTT.send(topic, message.retention, message.payload)
		}

		catbus.stopAllTimers()
		catbus.startAllTimers()

		// TODO: replace with proper channel signaling or sth.
		time.Sleep(1 * time.Second)

		got := fakeMQTT.payloadByTopic
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("[%d]: got %v, want %v", i, got, tt.want)
		}
	}
}

type (
	fakeMQTT struct {
		mqtt.Client

		callbackByTopic map[string]mqtt.MessageHandler
		payloadByTopic  map[string][]byte
	}

	fakeMessage struct {
		mqtt.Message

		topic    string
		retained bool
		payload  []byte
	}

	fakeToken struct{}
)

func (f *fakeMQTT) Publish(topic string, qos byte, retain bool, payload interface{}) mqtt.Token {
	bytes, ok := payload.([]byte)
	if !ok {
		panic(fmt.Sprintf("expected type []byte, got %v", reflect.TypeOf(payload)))
	}

	log.Printf("topic %q payload %s", topic, payload)
	f.payloadByTopic[topic] = bytes
	return &fakeToken{}
}
func (f *fakeMQTT) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token {
	f.callbackByTopic[topic] = callback

	return &fakeToken{}
}
func (f *fakeMQTT) send(topic string, retention Retention, payload []byte) {
	// if retention == Retain {
	// f.payloadByTopic[topic] = payload
	// }

	if callback, ok := f.callbackByTopic[topic]; ok {
		msg := &fakeMessage{
			topic:    topic,
			retained: bool(retention),
			payload:  payload,
		}
		callback(f, msg)
	}
}

func (f *fakeMessage) Topic() string {
	return f.topic
}
func (f *fakeMessage) Payload() []byte {
	return f.payload
}
func (f *fakeMessage) Retained() bool {
	return f.retained
}

func (_ *fakeToken) Wait() bool {
	return false
}
func (_ *fakeToken) WaitTimeout(_ time.Duration) bool {
	return false
}
func (_ *fakeToken) Error() error {
	return nil
}
