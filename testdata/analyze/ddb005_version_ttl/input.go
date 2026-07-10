package bad

//ddb:entity table=app type=order version=Rev ttl=Expires
//ddb:key pk="ORDER#{ID}"
type Order struct {
	ID      string `dynamodbav:"id"`
	Rev     string `dynamodbav:"rev"`
	Expires int32  `dynamodbav:"exp"`
}
