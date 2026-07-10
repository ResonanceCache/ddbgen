package bad

//ddb:entity table=app type=order
//ddb:key pk="ORDER#{ID}"
//ddb:key pk="ORDERS#{ID}"
type Order struct {
	ID string `dynamodbav:"id"`
}
