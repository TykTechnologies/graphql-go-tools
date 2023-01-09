package asyncapi

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/asyncapi/parser-go/pkg/parser"
	"github.com/buger/jsonparser"
)

var ErrMissingMessageObject = errors.New("missing message object")

const (
	ChannelsKey   string = "channels"
	SubscribeKey  string = "subscribe"
	MessageKey    string = "message"
	PayloadKey    string = "payload"
	PropertiesKey string = "properties"
	EnumKey       string = "enum"
)

type AsyncAPI struct {
	Channels map[string]*ChannelItem
	Servers  map[string]*Server
}

type ServerVariable struct {
}

type SecurityRequirement struct {
	Requirements map[string][]string
}

type ServerBindings struct {
}

type Server struct {
	URL             string
	Protocol        string
	ProtocolVersion string
	Description     string
	Variables       map[string]*ServerVariable
	Security        *SecurityRequirement
	Bindings        *ServerBindings
}

type ChannelItem struct {
	Message     *Message
	OperationID string
}

type Enum struct {
	Value     []byte
	ValueType jsonparser.ValueType
}

type Property struct {
	Description string
	Minimum     int
	Maximum     int
	Type        string
	Format      string
	Enum        []*Enum
}

type Payload struct {
	Type       string
	Properties map[string]*Property
}

type Message struct {
	Name        string
	Summary     string
	Title       string
	Description string
	Payload     *Payload
}

type walker struct {
	document *bytes.Buffer
	asyncapi *AsyncAPI
}

func extractString(key string, data []byte) (string, error) {
	value, dataType, _, err := jsonparser.Get(data, key)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return "", fmt.Errorf("key: %s is missing", key)
	}
	if dataType != jsonparser.String {
		return "", fmt.Errorf("key: %s has to be a string", key)
	}
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func extractInteger(key string, data []byte) (int, error) {
	value, dataType, _, err := jsonparser.Get(data, key)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return 0, fmt.Errorf("key: %s is missing", key)
	}
	if dataType != jsonparser.Number {
		return 0, fmt.Errorf("key: %s has to be a number", key)
	}
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(value))
}

func (w *walker) enterPropertyObject(channel, key, data []byte) error {
	property := &Property{}
	description, err := extractString("description", data)
	if err == nil {
		property.Description = description
	}

	format, err := extractString("format", data)
	if err == nil {
		property.Format = format
	}

	tpe, err := extractString("type", data)
	if err != nil {
		return err
	}
	property.Type = tpe

	minimum, err := extractInteger("minimum", data)
	if err == nil {
		property.Minimum = minimum
	}

	maximum, err := extractInteger("maximum", data)
	if err == nil {
		property.Maximum = maximum
	}

	_, err = jsonparser.ArrayEach(data, func(enumValue []byte, dataType jsonparser.ValueType, _ int, err error) {
		property.Enum = append(property.Enum, &Enum{
			Value:     enumValue,
			ValueType: dataType,
		})
	}, EnumKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		err = nil
	}
	if err != nil {
		return err
	}

	w.asyncapi.Channels[string(channel)].Message.Payload.Properties[string(key)] = property
	return nil
}

func (w *walker) enterPropertiesObject(channel, data []byte) error {
	propertiesValue, dataType, _, err := jsonparser.Get(data, PropertiesKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return fmt.Errorf("key: %s is missing", PropertiesKey)
	}
	if dataType != jsonparser.Object {
		return fmt.Errorf("key: %s has to be a JSON object", propertiesValue)
	}

	return jsonparser.ObjectEach(propertiesValue, func(key []byte, value []byte, dataType jsonparser.ValueType, _ int) error {
		return w.enterPropertyObject(channel, key, value)
	})
}

func (w *walker) enterPayloadObject(key, data []byte) error {
	payload, dataType, _, err := jsonparser.Get(data, PayloadKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return fmt.Errorf("key: %s is missing", PayloadKey)
	}
	if dataType != jsonparser.Object {
		return fmt.Errorf("key: %s has to be a JSON object", PayloadKey)
	}

	p := &Payload{Properties: make(map[string]*Property)}
	typeValue, err := extractString("type", payload)
	if err == nil {
		p.Type = typeValue
	}
	w.asyncapi.Channels[string(key)].Message.Payload = p

	return w.enterPropertiesObject(key, payload)
}

func (w *walker) enterMessageObject(key, data []byte) error {
	msg := &Message{}
	name, err := extractString("name", data)
	if err != nil {
		return err
	}
	msg.Name = name

	summary, err := extractString("summary", data)
	if err == nil {
		msg.Summary = summary
	}

	title, err := extractString("title", data)
	if err == nil {
		msg.Title = title
	}

	description, err := extractString("description", data)
	if err == nil {
		msg.Description = description
	}

	w.asyncapi.Channels[string(key)].Message = msg
	return w.enterPayloadObject(key, data)
}

func (w *walker) enterChannelItemObject(key []byte, data []byte) error {
	value, dataType, _, err := jsonparser.Get(data, SubscribeKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return nil
	}
	if err != nil {
		return err
	}

	if dataType != jsonparser.Object {
		return fmt.Errorf("%s has to be a JSON object", SubscribeKey)
	}

	messageValue, dataType, _, err := jsonparser.Get(value, MessageKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return fmt.Errorf("channel: %s: %w", key, ErrMissingMessageObject)
	}
	if err != nil {
		return err
	}

	if dataType != jsonparser.Object {
		return fmt.Errorf("%s has to be a JSON object", MessageKey)
	}

	operationID, err := extractString("operationId", value)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		err = nil
	}
	if err != nil {
		return err
	}

	w.asyncapi.Channels[string(key)] = &ChannelItem{OperationID: operationID}
	return w.enterMessageObject(key, messageValue)
}

func (w *walker) enterChannelObject() error {
	value, dataType, _, err := jsonparser.Get(w.document.Bytes(), ChannelsKey)
	if err != nil {
		return err
	}

	if dataType != jsonparser.Object {
		return fmt.Errorf("%s has to be a JSON object", ChannelsKey)
	}

	return jsonparser.ObjectEach(value, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		if dataType != jsonparser.Object {
			return fmt.Errorf("%s has to be a JSON object", key)
		}
		err = w.enterChannelItemObject(key, value)
		if err != nil {
			return err
		}
		return nil
	})
}

func (w *walker) enterServersObject() {

}

func ParseAsyncAPIDocument(input []byte) (*AsyncAPI, error) {
	r := bytes.NewBuffer(input)
	p, err := parser.New()
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(nil)
	err = p(r, buf)
	if err != nil {
		return nil, err
	}

	w := &walker{
		document: buf,
		asyncapi: &AsyncAPI{Channels: make(map[string]*ChannelItem)},
	}

	err = w.enterChannelObject()
	if err != nil {
		return nil, err
	}

	return w.asyncapi, nil
}
