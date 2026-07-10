package bad

//ddb:entity table=app type=user
//ddb:key pk="USER#{ID}" sk="PROFILE#{ID}"
type User struct {
	ID string `dynamodbav:"id"`
}

//ddb:entity table=app type=account
//ddb:key pk="USER#{Email}" sk="PROFILE#{Email}"
type Account struct {
	Email string `dynamodbav:"email"`
}
