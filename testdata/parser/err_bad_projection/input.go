package bad

//ddb:entity table=app type=order
//ddb:key pk="ORDER#{ID}"
//ddb:index name=GSI1 pk="STATUS#{Status}" projection=include
type Order struct {
	ID     string `dynamodbav:"id"`
	Status string `dynamodbav:"status"`
}
