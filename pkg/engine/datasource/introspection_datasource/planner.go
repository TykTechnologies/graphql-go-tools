package introspection_datasource

import (
	"encoding/json"
	"github.com/TykTechnologies/graphql-go-tools/pkg/engine/plan"
	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
)

type Planner struct {
	introspectionData *introspection.Data
	v                 *plan.Visitor
	rootField         int
	restrictionList   *RestrictionList
}

func (p *Planner) Register(visitor *plan.Visitor, dataSourceConfig plan.DataSourceConfiguration, _ bool) error {
	p.v = visitor
	visitor.Walker.RegisterEnterFieldVisitor(p)

	p.restrictionList = &RestrictionList{}
	return json.Unmarshal(dataSourceConfig.Custom, &p.restrictionList)
}

func (p *Planner) DownstreamResponseFieldAlias(_ int) (alias string, exists bool) {
	// the Introspection DataSourcePlanner doesn't rewrite upstream fields: skip
	return
}

func (p *Planner) DataSourcePlanningBehavior() plan.DataSourcePlanningBehavior {
	return plan.DataSourcePlanningBehavior{
		MergeAliasedRootNodes:      false,
		OverrideFieldPathFromAlias: false,
	}
}

func (p *Planner) EnterField(ref int) {
	p.rootField = ref
}

func (p *Planner) configureInput() string {
	fieldName := p.v.Operation.FieldNameString(p.rootField)

	return buildInput(fieldName)
}

func (p *Planner) ConfigureFetch() plan.FetchConfiguration {
	return plan.FetchConfiguration{
		Input: p.configureInput(),
		DataSource: &Source{
			introspectionData: p.introspectionData,
			restrictionList:   p.restrictionList,
		},
	}
}

func (p *Planner) ConfigureSubscription() plan.SubscriptionConfiguration {
	// the Introspection DataSourcePlanner doesn't have subscription
	return plan.SubscriptionConfiguration{}
}
