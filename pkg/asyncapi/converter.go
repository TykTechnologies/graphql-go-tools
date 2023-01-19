package asyncapi

import (
	"bytes"
	"encoding/json"
	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
	"github.com/buger/jsonparser"
)

type Converter struct {
	asyncapi          *AsyncAPI
	introspectionData *introspection.Data
	knownEnums        map[string]struct{}
	knownTypes        map[string]struct{}
}

func (c *Converter) importTypes() []introspection.FullType {
	fullTypes := make([]introspection.FullType, 0)
	for _, channelItem := range c.asyncapi.Channels {
		msg := channelItem.Message
		if _, ok := c.knownTypes[msg.Name]; ok {
			continue
		}
		ft := introspection.FullType{
			Kind:        introspection.OBJECT,
			Name:        msg.Name,
			Description: msg.Description,
		}
		for name, prop := range msg.Payload.Properties {
			if prop.Enum != nil && len(prop.Enum) != 0 {
				_, ok := c.knownEnums[name]
				if !ok {
					enumType := introspection.FullType{
						Kind: introspection.ENUM,
						Name: name,
					}
					for _, enum := range prop.Enum {
						if enum.ValueType == jsonparser.String {
							enumType.EnumValues = append(enumType.EnumValues, introspection.EnumValue{
								Name: string(enum.Value),
							})
						}
					}
					c.knownEnums[name] = struct{}{}
					fullTypes = append(fullTypes, enumType)
				}
			}
			copyName := name
			var f introspection.Field
			if prop.Enum == nil || len(prop.Enum) == 0 {
				f = introspection.Field{
					Name:        copyName,
					Description: prop.Description,
					Type: introspection.TypeRef{
						Kind: 0,
						Name: &prop.Type,
					},
				}
			} else {
				f = introspection.Field{
					Name:        copyName,
					Description: prop.Description,
					Type: introspection.TypeRef{
						Kind: 4,
						Name: &copyName,
					},
				}
			}
			ft.Fields = append(ft.Fields, f)
		}
		c.knownTypes[msg.Name] = struct{}{}
		fullTypes = append(fullTypes, ft)
	}
	return fullTypes
}

func (c *Converter) importSubscriptionType() introspection.FullType {
	subscriptionType := introspection.FullType{
		Kind: introspection.OBJECT,
		Name: "Subscription",
	}
	for _, channelItem := range c.asyncapi.Channels {
		typeName := channelItem.Message.Name
		f := introspection.Field{
			Name: channelItem.OperationID,
			Type: introspection.TypeRef{
				Kind: 3,
				Name: &typeName,
			},
		}
		for paramName, paramType := range channelItem.Parameters {
			n := paramType
			iv := introspection.InputValue{
				Name: paramName,
				Type: introspection.TypeRef{
					Kind: 0,
					Name: &n,
				},
			}
			f.Args = append(f.Args, iv)
		}
		subscriptionType.Fields = append(subscriptionType.Fields, f)
	}
	return subscriptionType
}

func ImportAsyncAPIDocumentByte(input []byte) (*ast.Document, operationreport.Report) {
	report := operationreport.Report{}
	asyncapi, err := ParseAsyncAPIDocument(input)
	if err != nil {
		report.AddInternalError(err)
		return nil, report
	}

	c := &Converter{
		asyncapi:   asyncapi,
		knownEnums: make(map[string]struct{}),
		knownTypes: make(map[string]struct{}),
	}

	data := introspection.Data{}
	data.Schema.SubscriptionType = &introspection.TypeName{
		Name: "Subscription",
	}
	data.Schema.Types = append(data.Schema.Types, c.importSubscriptionType())
	data.Schema.Types = append(data.Schema.Types, c.importTypes()...)
	outputPretty, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		report.AddInternalError(err)
		return nil, report
	}

	converter := introspection.JsonConverter{}
	buf := bytes.NewBuffer(outputPretty)
	doc, err := converter.GraphQLDocument(buf)
	if err != nil {
		report.AddInternalError(err)
		return nil, report
	}
	return doc, report
}
