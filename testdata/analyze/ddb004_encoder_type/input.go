package bad

//ddb:entity table=app type=order
//ddb:key pk="ORDER#{Num:pad8}" sk="AT#{When:rfc3339}"
type Order struct {
	Num  string `dynamodbav:"num"`
	When int64  `dynamodbav:"when"`
}
