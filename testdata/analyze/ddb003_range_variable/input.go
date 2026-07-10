package bad

//ddb:entity table=app type=doc
//ddb:key pk="DOC#{ID}" sk="REV#{Name}"
//ddb:pattern name=DocsAfter index=main pk="DOC#{ID}" sk.gt="REV#{Name}"
type Doc struct {
	ID   string `dynamodbav:"id"`
	Name string `dynamodbav:"name"`
}
