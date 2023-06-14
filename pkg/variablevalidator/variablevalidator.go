package variablevalidator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/astvisitor"
	"github.com/TykTechnologies/graphql-go-tools/pkg/graphqljsonschema"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
	"github.com/buger/jsonparser"
)

type VariableValidator struct {
	walker  *astvisitor.Walker
	visitor *validatorVisitor
}

func NewVariableValidator() *VariableValidator {
	walker := astvisitor.Walker{}
	validator := VariableValidator{
		walker: &walker,
		visitor: &validatorVisitor{
			Walker:           &walker,
			currentOperation: ast.InvalidRef,
		},
	}

	validator.walker.RegisterEnterDocumentVisitor(validator.visitor)
	validator.walker.RegisterEnterOperationVisitor(validator.visitor)
	validator.walker.RegisterLeaveOperationVisitor(validator.visitor)
	validator.walker.RegisterEnterVariableDefinitionVisitor(validator.visitor)

	return &validator
}

type validatorVisitor struct {
	*astvisitor.Walker

	operationName, variables []byte
	currentOperation         int
	operation, definition    *ast.Document
}

func (v *validatorVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation, v.definition = operation, definition
}

func (v *validatorVisitor) EnterVariableDefinition(ref int) {
	if v.currentOperation == ast.InvalidRef {
		return
	}
	typeRef := v.operation.VariableDefinitions[ref].Type

	variableName := v.operation.VariableDefinitionNameBytes(ref)
	variable, t, _, err := jsonparser.Get(v.variables, string(variableName))
	if t == jsonparser.NotExist && v.operation.TypeIsNonNull(typeRef) {
		v.StopWithExternalErr(operationreport.ErrVariableNotProvided(variableName, v.operation.VariableDefinitions[ref].VariableValue.Position))
		return
	}
	if err != nil {
		v.StopWithInternalErr(errors.New("error parsing variables"))
		return
	}

	if t == jsonparser.String {
		variable = []byte(fmt.Sprintf(`"%s"`, string(variable)))
	}

	jsonSchema := graphqljsonschema.FromTypeRef(v.operation, v.definition, typeRef)
	schemaValidator, err := graphqljsonschema.NewValidatorFromSchema(jsonSchema)
	if err != nil {
		v.StopWithInternalErr(err)
		return
	}
	if err := schemaValidator.Validate(context.Background(), variable); err != nil {
		v.StopWithExternalErr(operationreport.ErrVariableValidationFailed(variableName, err.Error(), v.operation.VariableDefinitions[ref].VariableValue.Position))
		return
	}
}

func (v *validatorVisitor) EnterOperationDefinition(ref int) {
	if len(v.operationName) == 0 {
		v.currentOperation = ref
		return
	}

	if bytes.Equal(v.operationName, v.operation.OperationDefinitionNameBytes(ref)) {
		v.currentOperation = ref
	}
}

func (v *validatorVisitor) LeaveOperationDefinition(ref int) {
	if v.currentOperation == ref {
		v.Stop()
	}
}

func (v *VariableValidator) Validate(operation, definition *ast.Document, operationName, variables []byte, report *operationreport.Report) {
	if v.visitor != nil {
		v.visitor.operationName = operationName
		v.visitor.variables = variables
	}

	v.walker.Walk(operation, definition, report)
}