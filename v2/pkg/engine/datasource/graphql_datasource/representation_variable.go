package graphql_datasource

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type objectFields struct {
	popOnField int
	fields     *[]*resolve.Field
}

func BuildRepresentationVariableNode(key, definition *ast.Document) (*resolve.Object, error) {
	walker := astvisitor.NewWalker(48)

	visitor := &representationVariableVisitor{
		Walker: &walker,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)

	var report operationreport.Report
	walker.Walk(key, definition, &report)
	if report.HasErrors() {
		return nil, report
	}

	return visitor.rootObject, nil
}

type representationVariableVisitor struct {
	*astvisitor.Walker
	key, definition *ast.Document

	currentFields []objectFields
	rootObject    *resolve.Object
}

func (v *representationVariableVisitor) EnterDocument(key, definition *ast.Document) {
	v.key = key
	v.definition = definition

	v.rootObject = &resolve.Object{
		Fields: []*resolve.Field{
			{
				Name: []byte("__typename"),
				Value: &resolve.String{
					Path: []string{"__typename"},
				},
			},
		},
	}

	v.currentFields = append(v.currentFields, objectFields{
		fields:     &v.rootObject.Fields,
		popOnField: -1,
	})
}

func (v *representationVariableVisitor) EnterField(ref int) {
	fieldName := v.key.FieldNameBytes(ref)

	fieldDefinition, ok := v.Walker.FieldDefinition(ref)
	if !ok {
		return
	}
	fieldDefinitionType := v.definition.FieldDefinitionType(fieldDefinition)

	currentField := &resolve.Field{
		Name:  fieldName,
		Value: v.resolveFieldValue(ref, fieldDefinitionType, true, []string{string(fieldName)}),
	}

	*v.currentFields[len(v.currentFields)-1].fields = append(*v.currentFields[len(v.currentFields)-1].fields, currentField)
}

func (v *representationVariableVisitor) LeaveField(ref int) {
	if v.currentFields[len(v.currentFields)-1].popOnField == ref {
		v.currentFields = v.currentFields[:len(v.currentFields)-1]
	}
}

func (v *representationVariableVisitor) resolveFieldValue(fieldRef, typeRef int, nullable bool, path []string) resolve.Node {
	ofType := v.definition.Types[typeRef].OfType

	switch v.definition.Types[typeRef].TypeKind {
	case ast.TypeKindNonNull:
		return v.resolveFieldValue(fieldRef, ofType, false, path)
	case ast.TypeKindList:
		listItem := v.resolveFieldValue(fieldRef, ofType, true, nil)
		return &resolve.Array{
			Nullable: nullable,
			Path:     path,
			Item:     listItem,
		}
	case ast.TypeKindNamed:
		typeName := v.definition.ResolveTypeNameString(typeRef)
		typeDefinitionNode, ok := v.definition.Index.FirstNodeByNameStr(typeName)
		if !ok {
			return &resolve.Null{}
		}
		switch typeDefinitionNode.Kind {
		case ast.NodeKindScalarTypeDefinition:
			switch typeName {
			case "String":
				return &resolve.String{
					Path:     path,
					Nullable: nullable,
				}
			case "Boolean":
				return &resolve.Boolean{
					Path:     path,
					Nullable: nullable,
				}
			case "Int":
				return &resolve.Integer{
					Path:     path,
					Nullable: nullable,
				}
			case "Float":
				return &resolve.Float{
					Path:     path,
					Nullable: nullable,
				}
			default:
				return &resolve.String{
					Path:     path,
					Nullable: nullable,
				}
			}
		case ast.NodeKindEnumTypeDefinition:
			return &resolve.String{
				Path:     path,
				Nullable: nullable,
			}
		case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
			object := &resolve.Object{
				Nullable: nullable,
				Path:     path,
				Fields:   []*resolve.Field{},
			}
			v.Walker.DefferOnEnterField(func() {
				v.currentFields = append(v.currentFields, objectFields{
					popOnField: fieldRef,
					fields:     &object.Fields,
				})
			})
			return object
		default:
			return &resolve.Null{}
		}
	default:
		return &resolve.Null{}
	}
}
