package bad

//ddb:entity table=app type=order
//ddb:key pk="ORDER#{ID}" sk="A#{ID}"
type Order struct {
	ID string `dynamodbav:"id"`
}

//ddb:entity table=app type=order
//ddb:key pk="LEGACY#{ID}" sk="B#{ID}"
type LegacyOrder struct {
	ID string `dynamodbav:"id"`
}
