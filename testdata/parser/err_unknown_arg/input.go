package bad

//ddb:entity table=app type=order shard=4
//ddb:key pk="ORDER#{ID}"
type Order struct {
	ID string `dynamodbav:"id"`
}
