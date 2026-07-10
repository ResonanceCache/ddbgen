package bad

//ddb:entity table=app type=doc
//ddb:key pk="DOC#{ID}" sk="REV#{Name}"
//ddb:pattern name=DocRange index=main pk="DOC#{ID}" sk.between
type Doc struct {
	ID   string `dynamodbav:"id"`
	Name string `dynamodbav:"name"`
}
