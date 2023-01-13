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
				Traits: []*OperationTrait{
					{
						Bindings: map[string]map[string]*Binding{
							"kafka": {
								"clientId": {
									Value:     []byte("my-app-id"),
									ValueType: jsonparser.String,
								},
								"groupId": {
									Value:     []byte("my-group-id"),
									ValueType: jsonparser.String,
								},
							},
						},
					},
				},
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
				Traits: []*OperationTrait{
					{
						Bindings: map[string]map[string]*Binding{
							"kafka": {
								"clientId": {
									Value:     []byte("my-app-id"),
									ValueType: jsonparser.String,
								},
								"groupId": {
									Value:     []byte("my-group-id"),
									ValueType: jsonparser.String,
								},
							},
						},
					},
				},
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
				Traits: []*OperationTrait{
					{
						Bindings: map[string]map[string]*Binding{
							"kafka": {
								"clientId": {
									Value:     []byte("my-app-id"),
									ValueType: jsonparser.String,
								},
								"groupId": {
									Value:     []byte("my-group-id"),
									ValueType: jsonparser.String,
								},
							},
						},
					},
				},
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
		Servers: map[string]*Server{
			"test": {
				URL:         "test.mykafkacluster.org:8092",
				Protocol:    "kafka-secure",
				Description: "Test broker",
				Bindings:    map[string]map[string]*Binding{},
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
				Traits:      []*OperationTrait{{Bindings: map[string]map[string]*Binding{}}},
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
				Traits:      []*OperationTrait{{Bindings: map[string]map[string]*Binding{}}},
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
				Traits:      []*OperationTrait{{Bindings: map[string]map[string]*Binding{}}},
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
		Servers: map[string]*Server{
			"test": {
				URL:         "test.mykafkacluster.org:8092",
				Protocol:    "kafka-secure",
				Description: "Test broker",
				Bindings: map[string]map[string]*Binding{
					"kafka": {
						"clientId": {
							Value:     []byte("my-app-id"),
							ValueType: jsonparser.String,
						},
						"groupId": {
							Value:     []byte("my-group-id"),
							ValueType: jsonparser.String,
						},
					},
				},
			},
			"test_oauth": {
				URL:         "test.mykafkacluster.org:8093",
				Protocol:    "kafka-secure",
				Description: "Test port for oauth",
				Bindings:    map[string]map[string]*Binding{},
				Security: []*SecurityRequirement{{
					Requirements: map[string][]string{
						"streetlights_auth": {"streetlights:write", "streetlights:read"},
					},
				}},
			},
		},
	}
	asyncapiDoc, err := os.ReadFile("./fixtures/streetlights-kafka-security.yaml")
	require.NoError(t, err)
	asyncapi, err := ParseAsyncAPIDocument(asyncapiDoc)
	require.NoError(t, err)
	require.Equal(t, expectedAsyncAPI, asyncapi)
}
