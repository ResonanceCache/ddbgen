package bad

//ddb:entity table=app type=doc version=Rev ttl=Expiry
//ddb:key pk="DOC#{ID}" sk="REV#{Rev:pad6}"
//ddb:index name=GSI1 pk="EXP#{Expiry:epoch}" sk="{ID}"
type Doc struct {
	ID     string `dynamodbav:"id"`
	Rev    int64  `dynamodbav:"rev"`
	Expiry int64  `dynamodbav:"expiry"`
}
