package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type primaryKeySet map[string]bool

type entitySet map[string]primaryKeySet

type entityValidator struct {
	document  *ast.Document
	entitySet entitySet
}

func (e *entityValidator) setDocument(document *ast.Document) {
	if e.document == nil {
		e.document = document
	}
}

func (e *entityValidator) getPrimaryKeys(name string, directiveRefs []int) (primaryKeySet, *operationreport.ExternalError) {
	primaryKeys := make(primaryKeySet)
	for _, directiveRef := range directiveRefs {
		if e.document.DirectiveNameString(directiveRef) != plan.FederationKeyDirectiveName {
			continue
		}
		directive := e.document.Directives[directiveRef]
		if len(directive.Arguments.Refs) != 1 {
			err := operationreport.ErrKeyDirectiveMustHaveSingleFieldsArgument(name)
			return nil, &err
		}
		argumentRef := directive.Arguments.Refs[0]
		if e.document.ArgumentNameString(argumentRef) != keyDirectiveArgument {
			err := operationreport.ErrKeyDirectiveMustHaveSingleFieldsArgument(name)
			return nil, &err
		}
		primaryKey := e.document.StringValueContentString(e.document.Arguments[argumentRef].Value.Ref)
		if primaryKey == "" {
			err := operationreport.ErrPrimaryKeyMustNotBeEmpty(name)
			return nil, &err
		}
		primaryKeys[primaryKey] = false
	}
	return primaryKeys, nil
}

func (e *entityValidator) getExtensionPrimaryKeys(name string, directiveRefs []int) (primaryKeySet, *operationreport.ExternalError) {
	primaryKeys := make(primaryKeySet)
	keyNumber := 0
	for _, directiveRef := range directiveRefs {
		if e.document.DirectiveNameString(directiveRef) != plan.FederationKeyDirectiveName {
			continue
		}
		if keyNumber > 1 {
			err := operationreport.ErrEntityExtensionMustHaveExactlyOnePrimaryKey(name)
			return nil, &err
		}
		directive := e.document.Directives[directiveRef]
		if len(directive.Arguments.Refs) != 1 {
			err := operationreport.ErrKeyDirectiveMustHaveSingleFieldsArgument(name)
			return nil, &err
		}
		argumentRef := directive.Arguments.Refs[0]
		if e.document.ArgumentNameString(argumentRef) != keyDirectiveArgument {
			err := operationreport.ErrKeyDirectiveMustHaveSingleFieldsArgument(name)
			return nil, &err
		}
		primaryKey := e.document.StringValueContentString(e.document.Arguments[argumentRef].Value.Ref)
		if primaryKey == "" {
			err := operationreport.ErrPrimaryKeyMustNotBeEmpty(name)
			return nil, &err
		}
		if _, exists := e.entitySet[name][primaryKey]; !exists {
			err := operationreport.ErrEntityPrimaryKeyMustExistAsField(name, primaryKey)
			return nil, &err
		}
		primaryKeys[primaryKey] = false
		keyNumber += 1
	}
	return primaryKeys, nil
}

func (e *entityValidator) validatePrimaryKeyReferences(name string, fieldRefs []int) *operationreport.ExternalError {
	primaryKeys := e.entitySet[name]
	fieldReferences := len(primaryKeys)
	if fieldReferences < 1 {
		return nil
	}
	for _, fieldRef := range fieldRefs {
		fieldName := e.document.FieldDefinitionNameString(fieldRef)
		isResolved, isPrimaryKey := primaryKeys[fieldName]
		if !isPrimaryKey {
			continue
		}
		if !e.isPrimaryKeyValid(fieldRef) {
			err := operationreport.ErrEntityPrimaryKeyMustBeValidType(name, fieldName)
			return &err
		}
		if !isResolved {
			primaryKeys[fieldName] = true
			fieldReferences -= 1
		}
		if fieldReferences == 0 {
			return nil
		}
	}
	for primaryKey, isResolved := range primaryKeys {
		if !isResolved {
			err := operationreport.ErrEntityPrimaryKeyMustExistAsField(name, primaryKey)
			return &err
		}
	}
	return nil
}

func (e *entityValidator) isEntityExtension(directiveRefs []int) bool {
	for _, directiveRef := range directiveRefs {
		if e.document.DirectiveNameString(directiveRef) == plan.FederationKeyDirectiveName {
			return true
		}
	}
	return false
}

func (e *entityValidator) validateExternalPrimaryKeys(name string, primaryKeys primaryKeySet, fieldRefs []int) *operationreport.ExternalError {
	fieldReferences := len(primaryKeys)
	if fieldReferences < 1 {
		err := operationreport.ErrEntityExtensionMustHaveKeyDirective(name)
		return &err
	}
	for _, fieldRef := range fieldRefs {
		fieldName := e.document.FieldDefinitionNameString(fieldRef)
		isExternalDirectiveResolved, isPrimaryKey := primaryKeys[fieldName]
		if !isPrimaryKey {
			continue
		}
		if err := e.validateExternalField(fieldRef, name, fieldName, isExternalDirectiveResolved, primaryKeys); err != nil {
			return err
		}
		fieldReferences -= 1
		if fieldReferences == 0 {
			return nil
		}
	}
	err := operationreport.ErrEntityExtensionPrimaryKeyMustExistAsField(name)
	return &err
}

func (e *entityValidator) validateExternalField(fieldRef int, name, fieldName string, isExternalDirectiveResolved bool, primaryKeys primaryKeySet) *operationreport.ExternalError {
	field := e.document.FieldDefinitions[fieldRef]
	for _, directiveRef := range field.Directives.Refs {
		if e.document.DirectiveNameString(directiveRef) != plan.FederationExternalDirectiveName {
			continue
		}
		if !isExternalDirectiveResolved {
			primaryKeys[fieldName] = true
		}
		return nil
	}
	err := operationreport.ErrEntityExtensionPrimaryKeyFieldReferenceMustHaveExternalDirective(name)
	return &err
}

func (e *entityValidator) isEntity(nameBytes []byte, hasDirectives bool, directiveRefs, fieldRefs []int) (bool, *operationreport.ExternalError) {
	name := string(nameBytes)
	if _, exists := e.entitySet[name]; !exists {
		if !hasDirectives || !e.isEntityExtension(directiveRefs) {
			return false, nil
		}
		err := operationreport.ErrExtensionWithKeyDirectiveMustExtendEntity(name)
		return false, &err
	}
	if !hasDirectives {
		err := operationreport.ErrEntityExtensionMustHaveKeyDirective(name)
		return false, &err
	}
	primaryKeys, err := e.getExtensionPrimaryKeys(name, directiveRefs)
	if err != nil {
		return false, err
	}
	err = e.validateExternalPrimaryKeys(name, primaryKeys, fieldRefs)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (e *entityValidator) isPrimaryKeyValid(fieldRef int) bool {
	fieldType := e.document.Types[e.document.FieldDefinitions[fieldRef].Type]
	for fieldType.OfType != -1 {
		fieldType = e.document.Types[fieldType.OfType]
	}
	fieldTypeNameBytes := e.document.Input.ByteSlice(fieldType.Name)
	nodes, exists := e.document.Index.NodesByNameBytes(fieldTypeNameBytes)
	if !exists {
		return true // we do not have the base schema at this point and so cannot validate
	}
	for i := range nodes {
		if nodes[i].Kind == ast.NodeKindInterfaceTypeDefinition || nodes[i].Kind == ast.NodeKindUnionTypeDefinition {
			return false
		}
	}
	return true
}
