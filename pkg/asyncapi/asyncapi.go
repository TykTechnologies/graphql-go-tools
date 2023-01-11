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
	ChannelsKey    = "channels"
	SubscribeKey   = "subscribe"
	MessageKey     = "message"
	PayloadKey     = "payload"
	PropertiesKey  = "properties"
	EnumKey        = "enum"
	ServersKey     = "servers"
	DescriptionKey = "description"
	NameKey        = "name"
	TitleKey       = "title"
	SummaryKey     = "summary"
	TypeKey        = "type"
	FormatKey      = "format"
	MinimumKey     = "minimum"
	MaximumKey     = "maximum"
	OperationIDKey = "operationId"
	SecurityKey    = "security"
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
	Security        []*SecurityRequirement
	Variables       map[string]*ServerVariable
	Bindings        *ServerBindings
}

type ChannelItem struct {
	Message     *Message
	OperationID string
	Servers     []string
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

func extractStringArray(key string, data []byte) ([]string, error) {
	var result []string
	_, err := jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, _ int, _ error) {
		result = append(result, string(value))
	}, key)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return result, nil
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
	description, err := extractString(DescriptionKey, data)
	if err == nil {
		property.Description = description
	}

	format, err := extractString(FormatKey, data)
	if err == nil {
		property.Format = format
	}

	tpe, err := extractString(TypeKey, data)
	if err != nil {
		return err
	}
	property.Type = tpe

	minimum, err := extractInteger(MinimumKey, data)
	if err == nil {
		property.Minimum = minimum
	}

	maximum, err := extractInteger(MaximumKey, data)
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
	typeValue, err := extractString(TypeKey, payload)
	if err == nil {
		p.Type = typeValue
	}
	w.asyncapi.Channels[string(key)].Message.Payload = p

	return w.enterPropertiesObject(key, payload)
}

func (w *walker) enterMessageObject(key, data []byte) error {
	msg := &Message{}
	name, err := extractString(NameKey, data)
	if err != nil {
		return err
	}
	msg.Name = name

	summary, err := extractString(SummaryKey, data)
	if err == nil {
		msg.Summary = summary
	}

	title, err := extractString(TitleKey, data)
	if err == nil {
		msg.Title = title
	}

	description, err := extractString(DescriptionKey, data)
	if err == nil {
		msg.Description = description
	}

	w.asyncapi.Channels[string(key)].Message = msg
	return w.enterPayloadObject(key, data)
}

func (w *walker) enterChannelItemObject(key []byte, data []byte) error {
	subscribeValue, dataType, _, err := jsonparser.Get(data, SubscribeKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return nil
	}
	if err != nil {
		return err
	}

	if dataType != jsonparser.Object {
		return fmt.Errorf("%s has to be a JSON object", SubscribeKey)
	}

	messageValue, dataType, _, err := jsonparser.Get(subscribeValue, MessageKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return fmt.Errorf("channel: %s: %w", key, ErrMissingMessageObject)
	}
	if err != nil {
		return err
	}

	if dataType != jsonparser.Object {
		return fmt.Errorf("%s has to be a JSON object", MessageKey)
	}

	operationID, err := extractString(OperationIDKey, subscribeValue)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		err = nil
	}
	if err != nil {
		return err
	}

	servers, err := extractStringArray(ServersKey, data)
	if err != nil {
		return err
	}
	channelItem := &ChannelItem{
		OperationID: operationID,
		Servers:     servers,
	}
	w.asyncapi.Channels[string(key)] = channelItem
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

func (w *walker) enterSecurityRequirementObject(key, data []byte, s *Server) error {
	sr := &SecurityRequirement{Requirements: make(map[string][]string)}

	_, err := jsonparser.ArrayEach(data, func(value3 []byte, dataType2 jsonparser.ValueType, _ int, _ error) {
		sr.Requirements[string(key)] = append(sr.Requirements[string(key)], string(value3))
	})
	if err != nil {
		return err
	}

	if len(sr.Requirements) > 0 {
		s.Security = append(s.Security, sr)
	}
	return nil
}

func (w *walker) enterSecurityObject(s *Server, data []byte) error {
	_, err := jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, _ int, err error) {
		err = jsonparser.ObjectEach(value, func(key []byte, value2 []byte, dataType2 jsonparser.ValueType, _ int) error {
			return w.enterSecurityRequirementObject(key, value2, s)
		})
	}, SecurityKey)
	return err
}

func (w *walker) enterServerObject(key, data []byte) error {
	s := &Server{}

	// Mandatory
	urlValue, err := extractString("url", data)
	if err != nil {
		return err
	}
	s.URL = urlValue

	protocolValue, err := extractString("protocol", data)
	if err != nil {
		return err
	}
	s.Protocol = protocolValue

	// Not mandatory
	protocolVersionValue, err := extractString("protocolVersion", data)
	if err == nil {
		s.ProtocolVersion = protocolVersionValue
	}
	descriptionValue, err := extractString("description", data)
	if err == nil {
		s.Description = descriptionValue
	}

	err = w.enterSecurityObject(s, data)
	if err != nil {
		return err
	}

	w.asyncapi.Servers[string(key)] = s
	return nil
}

func (w *walker) enterServersObject() error {
	serverValue, dataType, _, err := jsonparser.Get(w.document.Bytes(), ServersKey)
	if err != nil {
		return err
	}
	if dataType != jsonparser.Object {
		return fmt.Errorf("%s has to be a JSON object", ServersKey)
	}
	return jsonparser.ObjectEach(serverValue, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		return w.enterServerObject(key, value)
	})
}

func ParseAsyncAPIDocument(input []byte) (*AsyncAPI, error) {
	r := bytes.NewBuffer(input)
	asyncAPIParser, err := parser.New()
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(nil)
	err = asyncAPIParser(r, buf)
	if err != nil {
		return nil, err
	}

	w := &walker{
		document: buf,
		asyncapi: &AsyncAPI{
			Channels: make(map[string]*ChannelItem),
			Servers:  make(map[string]*Server),
		},
	}

	err = w.enterChannelObject()
	if err != nil {
		return nil, err
	}

	err = w.enterServersObject()
	if err != nil {
		return nil, err
	}

	return w.asyncapi, nil
}
