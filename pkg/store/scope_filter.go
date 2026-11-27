package store

import (
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"

	"github.com/openshift-online/rosa-seaplane-zoa/pkg/catalog"
)

func executionScopeCondition(scope string) expression.ConditionBuilder {
	values := catalog.ExecutionScopeDynamoValues(scope)
	if len(values) == 0 {
		return expression.Name("scope").Equal(expression.Value(scope))
	}
	if len(values) == 1 {
		return expression.Name("scope").Equal(expression.Value(values[0]))
	}
	operands := make([]expression.OperandBuilder, len(values))
	for i, v := range values {
		operands[i] = expression.Value(v)
	}
	return expression.Name("scope").In(operands[0], operands[1:]...)
}
