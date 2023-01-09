package asyncapi

import (
	"os"
	"testing"

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/require"
)

func TestAsyncAPIStreetLightsKafka(t *testing.T) {
	expectedAsyncAPI := &AsyncAPI{
		Channels: map[string]*ChannelItem{
			"smartylighting.streetlights.1.0.action.{streetlightId}.dim": {
				OperationID: "dimLight",
				Message: &Message{
					Name:    "dimLight",
					Summary: "Command a particular streetlight to dim the lights.",
					Title:   "Dim light",
					Payload: &Payload{
						Type: "object",
						Properties: map[string]*Property{
							"percentage": {
								Description: "Percentage to which the light should be dimmed to.",
								Minimum:     0,
								Maximum:     100,
								Type:        "integer",
							},
							"sentAt": {
								Description: "Date and time when the message was sent.",
								Type:        "string",
								Format:      "date-time",
							},
						},
					},
				},
			},
			"smartylighting.streetlights.1.0.action.{streetlightId}.turn.off": {
				OperationID: "turnOff",
				Message: &Message{
					Name:    "turnOnOff",
					Summary: "Command a particular streetlight to turn the lights on or off.",
					Title:   "Turn on/off",
					Payload: &Payload{
						Type: "object",
						Properties: map[string]*Property{
							"command": {
								Description: "Whether to turn on or off the light.",
								Type:        "string",
								Enum: []*Enum{
									{
										Value:     []byte("on"),
										ValueType: jsonparser.String,
									},
									{
										Value:     []byte("off"),
										ValueType: jsonparser.String,
									},
								},
							},
							"sentAt": {
								Description: "Date and time when the message was sent.",
								Type:        "string",
								Format:      "date-time",
							},
						},
					},
				},
			},
			"smartylighting.streetlights.1.0.action.{streetlightId}.turn.on": {
				OperationID: "turnOn",
				Message: &Message{
					Name:    "turnOnOff",
					Summary: "Command a particular streetlight to turn the lights on or off.",
					Title:   "Turn on/off",
					Payload: &Payload{
						Type: "object",
						Properties: map[string]*Property{
							"command": {
								Description: "Whether to turn on or off the light.",
								Type:        "string",
								Enum: []*Enum{
									{
										Value:     []byte("on"),
										ValueType: jsonparser.String,
									},
									{
										Value:     []byte("off"),
										ValueType: jsonparser.String,
									},
								},
							},
							"sentAt": {
								Description: "Date and time when the message was sent.",
								Type:        "string",
								Format:      "date-time",
							},
						},
					},
				},
			},
		},
	}
	asyncapiDoc, err := os.ReadFile("./fixtures/streetlights-kafka.yaml")
	require.NoError(t, err)
	asyncapi, err := ParseAsyncAPIDocument(asyncapiDoc)
	require.NoError(t, err)
	require.Equal(t, expectedAsyncAPI, asyncapi)
}

func TestAsyncAPIStreetLightsKafkaSecurity(t *testing.T) {
	expectedAsyncAPI := &AsyncAPI{
		Channels: map[string]*ChannelItem{
			"smartylighting.streetlights.1.0.action.{streetlightId}.dim": {
				OperationID: "dimLight",
				Servers:     []string{"test_oauth"},
				Message: &Message{
					Name:    "dimLight",
					Summary: "Command a particular streetlight to dim the lights.",
					Title:   "Dim light",
					Payload: &Payload{
						Type: "object",
						Properties: map[string]*Property{
							"percentage": {
								Description: "Percentage to which the light should be dimmed to.",
								Minimum:     0,
								Maximum:     100,
								Type:        "integer",
							},
							"sentAt": {
								Description: "Date and time when the message was sent.",
								Type:        "string",
								Format:      "date-time",
							},
						},
					},
				},
			},
			"smartylighting.streetlights.1.0.action.{streetlightId}.turn.off": {
				OperationID: "turnOff",
				Servers:     []string{"test_oauth"},
				Message: &Message{
					Name:    "turnOnOff",
					Summary: "Command a particular streetlight to turn the lights on or off.",
					Title:   "Turn on/off",
					Payload: &Payload{
						Type: "object",
						Properties: map[string]*Property{
							"command": {
								Description: "Whether to turn on or off the light.",
								Type:        "string",
								Enum: []*Enum{
									{
										Value:     []byte("on"),
										ValueType: jsonparser.String,
									},
									{
										Value:     []byte("off"),
										ValueType: jsonparser.String,
									},
								},
							},
							"sentAt": {
								Description: "Date and time when the message was sent.",
								Type:        "string",
								Format:      "date-time",
							},
						},
					},
				},
			},
			"smartylighting.streetlights.1.0.action.{streetlightId}.turn.on": {
				OperationID: "turnOn",
				Servers:     []string{"test_oauth"},
				Message: &Message{
					Name:    "turnOnOff",
					Summary: "Command a particular streetlight to turn the lights on or off.",
					Title:   "Turn on/off",
					Payload: &Payload{
						Type: "object",
						Properties: map[string]*Property{
							"command": {
								Description: "Whether to turn on or off the light.",
								Type:        "string",
								Enum: []*Enum{
									{
										Value:     []byte("on"),
										ValueType: jsonparser.String,
									},
									{
										Value:     []byte("off"),
										ValueType: jsonparser.String,
									},
								},
							},
							"sentAt": {
								Description: "Date and time when the message was sent.",
								Type:        "string",
								Format:      "date-time",
							},
						},
					},
				},
			},
		},
	}
	asyncapiDoc, err := os.ReadFile("./fixtures/streetlights-kafka-security.yaml")
	require.NoError(t, err)
	asyncapi, err := ParseAsyncAPIDocument(asyncapiDoc)
	require.NoError(t, err)
	require.Equal(t, expectedAsyncAPI, asyncapi)
}
