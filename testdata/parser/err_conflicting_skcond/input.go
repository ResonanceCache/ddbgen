package bad

//ddb:entity table=app type=order
//ddb:key pk="TENANT#{TenantID}" sk="ORDER#{ID}"
//ddb:pattern name=Orders index=main pk="TENANT#{TenantID}" sk.begins="ORDER#" sk.eq="ORDER#{ID}"
type Order struct {
	TenantID string `dynamodbav:"tenant_id"`
	ID       string `dynamodbav:"id"`
}
