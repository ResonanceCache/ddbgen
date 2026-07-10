package bad

//ddb:entity table=app type=order
//ddb:key pk="ORDER#{ID}" sk="A#{ID}"
//ddb:pattern name=ByID index=main pk="ORDER#{ID}"
type Order struct {
	ID string `dynamodbav:"id"`
}

//ddb:entity table=app type=invoice
//ddb:key pk="INVOICE#{ID}" sk="B#{ID}"
//ddb:pattern name=ByID index=main pk="INVOICE#{ID}"
type Invoice struct {
	ID string `dynamodbav:"id"`
}
